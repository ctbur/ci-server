package build

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
)

func RunBuilder() error {
	paramsJSON := os.Getenv("CI_BUILDER_PARAMS")
	if paramsJSON == "" {
		return errors.New("missing CI_BUILDER_PARAMS for builder")
	}

	var p BuilderParams
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
		return fmt.Errorf("failed to unmarshal build params JSON '%s': %w", paramsJSON, err)
	}

	// Run the build
	exitCode, err := build(*slog.Default(), p)
	if err != nil {
		return fmt.Errorf("builder failed: %w", err)
	}

	// Write exit code to file
	exitCodeFile := getExitCodeFile(p.DataDir, p.Build.ID)
	if err := os.WriteFile(exitCodeFile, []byte(strconv.Itoa(exitCode)), 0o600); err != nil {
		return fmt.Errorf("failed to write exit code to file '%s': %w", exitCodeFile, err)
	}
	slog.Info("Wrote exit code to file", slog.Int("exit_code", exitCode))

	return nil
}

func build(log slog.Logger, p BuilderParams) (int, error) {
	buildDir := getBuildDir(p.DataDir, p.Build.ID)

	if p.Build.CacheID != nil {
		log.Info("Copying cache", slog.Uint64("cache_id", *p.Build.CacheID))
		cacheDir := getBuildDir(p.DataDir, *p.Build.CacheID)
		// Copying will create the build dir
		if err := copyDirs(cacheDir, buildDir); err != nil {
			return 0, fmt.Errorf(
				"failed to copy repo cache dir '%s' to build dir '%s': %w",
				cacheDir, buildDir, err,
			)
		}
	} else {
		// Create an empty build dir
		if err := os.Mkdir(buildDir, 0o700); err != nil {
			return 0, fmt.Errorf("failed to create build dir: %w", err)
		}
	}

	// Get absoluate build dir path for sandbox parameters
	absBuildDir, err := filepath.Abs(buildDir)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve build dir path: %w", err)
	}

	// Checkout
	checkoutDir := path.Join(absBuildDir, p.Repo.Owner, p.Repo.Name)
	if err := os.MkdirAll(checkoutDir, 0o700); err != nil {
		return 0, fmt.Errorf("failed to create build dir: %w", err)
	}
	err = checkout(p.Repo.Owner, p.Repo.Name, p.Build.CommitSHA, checkoutDir)
	if err != nil {
		return 0, err
	}

	// Run build command
	logFilePath := getLogFile(p.DataDir, p.Build.ID)
	// sec: Path is from a trusted user
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	log.Info("Starting build...", slog.Any("command", p.Repo.BuildCmd))
	buildCmd := buildSandboxedCommand(
		absBuildDir, checkoutDir, p.Repo.BuildCmd, p.Repo.EnvVars, p.Repo.BuildSecrets,
	)
	exitCode, err := runWithLogs(buildCmd, logFile)
	if err != nil {
		return 0, err
	}
	log.Info("Finished build command", slog.Int("exit_code", exitCode))

	// Decide whether to run deploy command
	// Don't run deploy if not requested
	if !p.RunDeploy {
		slog.Info("No deploy is requested")
		return exitCode, nil
	}
	// Don't run deploy if build failed
	if exitCode != 0 {
		slog.Info("Deploy is requested but the build failed")
		return exitCode, nil
	}
	// Don't run deploy if there is no command
	if len(p.Repo.DeployCmd) == 0 {
		slog.Info("Deploy is requested but no deploy command is configured")
		return 0, nil
	}

	// Run deploy command
	log.Info("Starting deploy...", slog.Any("command", p.Repo.DeployCmd))
	deployCmd := buildSandboxedCommand(
		absBuildDir, checkoutDir, p.Repo.DeployCmd, p.Repo.EnvVars, p.Repo.DeploySecrets,
	)
	exitCode, err = runWithLogs(deployCmd, logFile)
	if err != nil {
		return 0, err
	}
	log.Info("Finished deploy command", slog.Int("exit_code", exitCode))

	return exitCode, nil
}

func copyDirs(src, dst string) error {
	// Use cp -a for archive copy (preserving most (all?) attributes and symlinks)
	cpCmd := exec.Command("cp", "-a", src, dst)

	out := &bytes.Buffer{}
	cpCmd.Stdout = out
	cpCmd.Stderr = out

	if err := cpCmd.Run(); err != nil {
		return fmt.Errorf("failed to copy dirs: w%\n\ncp output:\n%s", err, out)
	}
	return nil
}

func checkout(owner, name, commitSHA, targetDir string) error {
	initCmd := exec.Command("git", "-C", targetDir, "init", "-q")
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to init repo at '%s': %w", targetDir, err)
	}

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	// sec: Path comes from a trusted user, other args are not security critical
	cloneCmd := exec.Command("git", "-C", targetDir, "fetch", "--depth=1", cloneURL, commitSHA) // #nosec G204
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch repo at '%s': %w", cloneURL, err)
	}

	// Check out repo files to build dir
	// sec: Path comes from a trusted user, other args are not security critical
	checkoutCmd := exec.Command(
		"git",
		"--git-dir", fmt.Sprintf("%s/.git", targetDir),
		"--work-tree", targetDir,
		"checkout", commitSHA, "--", ".",
	) // #nosec G204
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit for '%s': %w", cloneURL, err)
	}

	return nil
}

func buildSandboxedCommand(
	absBuildDir string,
	checkoutDir string,
	cmd []string,
	env map[string]string,
	secrets map[string]string,
) *exec.Cmd {
	// Run command in bubblewrap sandbox
	var bwrapSandbox = []string{
		"bwrap",
		"--die-with-parent",
		"--unshare-all", "--share-net",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--bind", absBuildDir, absBuildDir,
		// Run all commands in the dir where we check out the repo
		"--chdir", checkoutDir,
	}
	cmd = append(bwrapSandbox, cmd...)
	// sec: Command is from a trusted user
	sandboxCmd := exec.Command(cmd[0], cmd[1:]...) // #nosec G204

	// Add secrets and env vars to the environment
	var cmdEnv []string
	for secret, value := range secrets {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", secret, value))
	}
	for name, value := range env {
		cmdEnv = append(cmdEnv, fmt.Sprintf("%s=%s", name, value))
	}

	// Add default env vars
	cmdEnv = append(cmdEnv,
		"CI=true",
		// Pass along PATH variable
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		// Set build dir as HOME
		fmt.Sprintf("HOME=%s", absBuildDir),
	)

	sandboxCmd.Env = cmdEnv
	return sandboxCmd
}

func runWithLogs(
	cmd *exec.Cmd,
	logFile *os.File,
) (int, error) {
	logChan := make(chan store.LogEntry, 100)
	errChan := make(chan error, 3)
	var logReaderWaitGroup sync.WaitGroup

	// Read stdout and stderr
	readLogStream := func(stream store.LogStream, reader *io.PipeReader) {
		defer logReaderWaitGroup.Done()
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			logChan <- store.LogEntry{
				Stream:    stream,
				Timestamp: time.Now(),
				Text:      line,
			}
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("failed to read build logs: %w", err)
		}
	}

	outReader, outWriter := io.Pipe()
	cmd.Stdout = outWriter
	logReaderWaitGroup.Add(1)
	go readLogStream(store.LogStreamStdout, outReader)

	errReader, errWriter := io.Pipe()
	cmd.Stderr = errWriter
	logReaderWaitGroup.Add(1)
	go readLogStream(store.LogStreamStderr, errReader)

	// Write logs to file
	logDoneChan := make(chan struct{})
	go func() {
		defer func() { logDoneChan <- struct{}{} }()

		encoder := json.NewEncoder(logFile)
		for logEntry := range logChan {
			if err := encoder.Encode(logEntry); err != nil {
				errChan <- fmt.Errorf("failed to write log entry to file: %w", err)
				return
			}
		}
	}()

	// Run and wait for the command
	if err := cmd.Start(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return 0, fmt.Errorf("failed to start build command: %w", err)
		}
	}

	errs := []error{}
	if err := cmd.Wait(); err != nil && err.(*exec.ExitError) == nil {
		errs = append(errs, fmt.Errorf("failed to execute build command: %w", err))
	}

	// Wait for log readers to finish
	_ = outWriter.Close()
	_ = errWriter.Close()
	logReaderWaitGroup.Wait()
	close(logChan)

	// Wait for log writer to finish
	<-logDoneChan

	// Collect errors
LOOP:
	for {
		select {
		case err := <-errChan:
			errs = append(errs, err)
		default:
			break LOOP
		}
	}

	return cmd.ProcessState.ExitCode(), errors.Join(errs...)
}
