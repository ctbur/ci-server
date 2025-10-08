package build

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
)

type BuilderParams struct {
	DataDir       string            `json:"data_dir"`
	BuildID       uint64            `json:"build_id"`
	CacheID       *uint64           `json:"cache_id"`
	RepoOwner     string            `json:"repo_owner"`
	RepoName      string            `json:"repo_name"`
	CommitSHA     string            `json:"commit_sha"`
	EnvVars       map[string]string `json:"env_vars"`
	BuildCmd      []string          `json:"build_cmd"`
	BuildSecrets  map[string]string `json:"build_secrets"`
	DeployCmd     []string          `json:"deploy_cmd"`
	DeploySecrets map[string]string `json:"deploy_secrets"`
}

// Create a new builder process by starting the same executable as the current
// process, but with the "builder" argument.
func startBuilder(params BuilderParams) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path: %w", err)
	}

	paramsJSON, err := json.Marshal(&params)
	if err != nil {
		return 0, fmt.Errorf("failed to build params to JSON: %w", err)
	}

	builderCmd := exec.Command(exe, "builder")
	builderCmd.Env = []string{
		// Pass along PATH variable
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		// Add builder params to env var
		fmt.Sprintf("CI_BUILDER_PARAMS=%s", paramsJSON),
		// Set build ID separately as it helps with identification of the builder process
		fmt.Sprintf("CI_BUILDER_BUILD_ID=%d", params.BuildID),
	}

	// Keep builder running independently of server
	builderCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	logFile := getBuilderLogFile(params.DataDir, params.BuildID)
	outFile, _ := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o644)
	builderCmd.Stdout = outFile
	builderCmd.Stderr = outFile

	if err := builderCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start builder process: %w", err)
	}

	return builderCmd.Process.Pid, nil
}

func RunBuilder() error {
	paramsJSON := os.Getenv("CI_BUILDER_PARAMS")
	if paramsJSON == "" {
		return errors.New("missing CI_BUILDER_PARAMS for builder")
	}

	var params BuilderParams
	if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
		return fmt.Errorf("failed to unmarshal build params JSON '%s': %w", paramsJSON, err)
	}

	// Run the build
	exitCode, err := build(*slog.Default(), params)
	if err != nil {
		return fmt.Errorf("builder failed: %w", err)
	}

	// Write exit code to file
	exitCodeFile := getExitCodeFile(params.DataDir, params.BuildID)
	if err := os.WriteFile(exitCodeFile, []byte(strconv.Itoa(exitCode)), 0o644); err != nil {
		return fmt.Errorf("failed to write exit code to file '%s': %w", exitCodeFile, err)
	}
	slog.Info("Wrote exit code to file", slog.Int("exit_code", exitCode))

	return nil
}

// isBuilderRunning checks if a builder process is still running. The process is
// identified using PID and build ID in env. This is to protect against PID
// reuse.
func isBuilderRunning(pid int, buildID uint64) bool {
	// Check if the process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Check if process is running by sending signal 0 - necessary on Unix
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}

	// Check if the BUILD_ID environment variable matches
	cmdlinePath := fmt.Sprintf("/proc/%d/environ", pid)
	data, err := os.ReadFile(cmdlinePath)
	if err != nil {
		return false
	}

	envVars := strings.Split(string(data), "\000")
	buildEnvVar := fmt.Sprintf("CI_BUILDER_BUILD_ID=%d", buildID)
	if !slices.Contains(envVars, buildEnvVar) {
		return false
	}

	return true
}

func build(log slog.Logger, p BuilderParams) (int, error) {
	buildDir := getBuildDir(p.DataDir, p.BuildID)

	if err := os.Mkdir(buildDir, 0o700); err != nil {
		return 0, fmt.Errorf("failed to create build dir: %w", err)
	}

	if p.CacheID != nil {
		log.Info("Copying cache", slog.Uint64("cache_id", *p.CacheID))
		cacheDir := getBuildDir(p.DataDir, *p.CacheID)
		if err := os.CopyFS(buildDir, os.DirFS(cacheDir)); err != nil {
			return 0, fmt.Errorf(
				"failed to copy repo cache dir '%s' to build dir '%s'",
				cacheDir, buildDir,
			)
		}
	}

	if err := checkout(p.RepoOwner, p.RepoName, p.CommitSHA, buildDir); err != nil {
		return 0, err
	}

	// Get absoluate build dir path for sandbox parameters
	absBuildDir, err := filepath.Abs(buildDir)
	if err != nil {
		return 0, fmt.Errorf("failed to resolve build dir path: %w", err)
	}

	// Run build command
	logFilePath := getLogFile(p.DataDir, p.BuildID)
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	log.Info("Starting build...", slog.Any("command", p.BuildCmd))
	buildCmd := buildSandboxedCommand(absBuildDir, p.BuildCmd, p.EnvVars, p.BuildSecrets)
	exitCode, err := runWithLogs(buildCmd, logFile)
	if err != nil {
		return 0, err
	}

	// Run deploy command - only if it exists and the build command was successful
	if exitCode != 0 || len(p.DeployCmd) == 0 {
		return exitCode, nil
	}
	log.Info("Starting deploy...", slog.Any("command", p.DeployCmd))
	deployCmd := buildSandboxedCommand(absBuildDir, p.DeployCmd, p.EnvVars, p.DeploySecrets)
	exitCode, err = runWithLogs(deployCmd, logFile)
	if err != nil {
		return 0, err
	}

	return exitCode, nil
}

func checkout(owner, name, commitSHA, targetDir string) error {
	initCmd := exec.Command("git", "-C", targetDir, "init", "-q")
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to init repo at '%s': %v", targetDir, err)
	}

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	cloneCmd := exec.Command("git", "-C", targetDir, "fetch", "--depth=1", cloneURL, commitSHA)
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch repo at '%s': %v", cloneURL, err)
	}

	// Check out repo files to build dir
	checkoutCmd := exec.Command(
		"git",
		"--git-dir", fmt.Sprintf("%s/.git", targetDir),
		"--work-tree", targetDir,
		"checkout", commitSHA, "--", ".",
	)
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit for '%s': %v", cloneURL, err)
	}

	return nil
}

func buildSandboxedCommand(
	absBuildDir string,
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
		"--chdir", absBuildDir,
	}
	cmd = append(bwrapSandbox, cmd...)
	sandboxCmd := exec.Command(cmd[0], cmd[1:]...)

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
	outWriter.Close()
	errWriter.Close()
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
