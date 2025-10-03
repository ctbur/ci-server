package build

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
)

// Create a new builder process by starting the same executable as the current
// process, but with the "builder" argument. All arguments are passed as
// environment variables.
func startBuilder(
	dataDir string,
	buildID uint64,
	repoOwner string,
	repoName string,
	commitSHA string,
	cmd []string,
	cacheID *uint64,
) (int, error) {
	// TODO: point to log file for builder itself

	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path: %w", err)
	}

	builderCmd := exec.Command(
		exe,
		"builder",
	)
	var env []string
	env = append(env, fmt.Sprintf("CI_BUILDER_DATA_DIR=%s", dataDir))
	env = append(env, fmt.Sprintf("CI_BUILDER_BUILD_ID=%d", buildID))
	env = append(env, fmt.Sprintf("CI_BUILDER_REPO_OWNER=%s", repoOwner))
	env = append(env, fmt.Sprintf("CI_BUILDER_REPO_NAME=%s", repoName))
	env = append(env, fmt.Sprintf("CI_BUILDER_COMMIT_SHA=%s", commitSHA))
	cmdSer, err := json.Marshal(cmd)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal string list to JSON: %w", err)
	}
	env = append(env, fmt.Sprintf("CI_BUILDER_CMD=%s", cmdSer))
	if cacheID != nil {
		env = append(env, fmt.Sprintf("CI_BUILDER_CACHE_ID=%d", *cacheID))
	}
	builderCmd.Env = append(os.Environ(), env...)

	if err := builderCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start builder process: %w", err)
	}

	return builderCmd.Process.Pid, nil
}

func RunBuilderFromEnv() error {
	// Read environment variables
	dataDir := os.Getenv("CI_BUILDER_DATA_DIR")
	buildIDStr := os.Getenv("CI_BUILDER_BUILD_ID")
	repoOwner := os.Getenv("CI_BUILDER_REPO_OWNER")
	repoName := os.Getenv("CI_BUILDER_REPO_NAME")
	commitSHA := os.Getenv("CI_BUILDER_COMMIT_SHA")
	cmdStr := os.Getenv("CI_BUILDER_CMD")
	cacheIDStr := os.Getenv("CI_BUILDER_CACHE_ID")

	if dataDir == "" || buildIDStr == "" || repoOwner == "" || repoName == "" ||
		commitSHA == "" || cmdStr == "" {
		return errors.New("missing required environment variables for builder")
	}

	buildID, err := strconv.ParseUint(buildIDStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse build ID '%s': %w", buildIDStr, err)
	}

	var cacheID *uint64
	if cacheIDStr != "" {
		parsedCacheID, err := strconv.ParseUint(cacheIDStr, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse cache ID '%s': %w", cacheIDStr, err)
		}
		cacheID = &parsedCacheID
	}

	var cmd []string
	if err := json.Unmarshal([]byte(cmdStr), &cmd); err != nil {
		return fmt.Errorf("failed to unmarshal command JSON '%s': %w", cmdStr, err)
	}
	if len(cmd) == 0 {
		return errors.New("empty command for builder")
	}

	// Run the build
	exitCode, err := build(dataDir, buildID, repoOwner, repoName, commitSHA, cmd, cacheID)
	if err != nil {
		return fmt.Errorf("builder failed: %w", err)
	}

	// Write exit code to file
	exitCodeFile := getExitCodeFile(dataDir, buildID)
	if err := os.WriteFile(exitCodeFile, []byte(strconv.Itoa(exitCode)), 0o644); err != nil {
		return fmt.Errorf("failed to write exit code to file '%s': %w", exitCodeFile, err)
	}

	return nil
}

// isBuilderRunning checks if a builder process is still running.
// The process is identified using PID and BUILD_ID in env. This
// is to protect against PID reuse.
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
	if slices.Contains(envVars, buildEnvVar) {
		return false
	}

	return true
}

func build(
	dataDir string,
	buildID uint64,
	repoOwner string,
	repoName string,
	commitSHA string,
	cmd []string,
	cacheID *uint64,
) (int, error) {
	buildDir := getBuildDir(dataDir, buildID)

	if cacheID != nil {
		cacheDir := getCacheDir(dataDir, repoOwner, repoName, *cacheID)
		if err := os.CopyFS(buildDir, os.DirFS(cacheDir)); err != nil {
			return 0, fmt.Errorf(
				"failed to copy repo cache dir '%s' to build dir '%s'",
				cacheDir, buildDir,
			)
		}
	}

	if err := checkout(repoOwner, repoName, commitSHA, buildDir); err != nil {
		return 0, err
	}

	logFile := getLogFile(dataDir, buildID)
	exitCode, err := runBuildCommand(buildDir, cmd, logFile)
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

func runBuildCommand(
	dir string,
	cmd []string,
	logFile string,
) (int, error) {
	buildCmd := exec.Command(cmd[0], cmd[1:]...)
	buildCmd.Dir = dir

	if err := buildCmd.Start(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return 0, fmt.Errorf("failed to start build command: %w", err)
		}
	}

	logChan := make(chan store.LogEntry, 100)
	errChan := make(chan error, 3)
	var logReaderWaitGroup sync.WaitGroup

	// Read stdout and stderr
	readLogStream := func(
		stream store.LogStream,
		reader *io.PipeReader,
		writer *io.PipeWriter,
	) {
		defer logReaderWaitGroup.Done()
		defer reader.Close()
		defer writer.Close()

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
	buildCmd.Stdout = outWriter
	logReaderWaitGroup.Add(1)
	go readLogStream(store.LogStreamStdout, outReader, outWriter)

	errReader, errWriter := io.Pipe()
	buildCmd.Stderr = errWriter
	logReaderWaitGroup.Add(1)
	go readLogStream(store.LogStreamStderr, errReader, errWriter)

	// Write logs to file
	logDoneChan := make(chan struct{})
	go func() {
		defer func() { logDoneChan <- struct{}{} }()
		logF, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			errChan <- fmt.Errorf("failed to open log file '%s': %w", logFile, err)
			return
		}
		defer logF.Close()

		encoder := json.NewEncoder(logF)
		for logEntry := range logChan {
			if err := encoder.Encode(logEntry); err != nil {
				errChan <- fmt.Errorf("failed to write log entry to file: %w", err)
				return
			}
		}
	}()

	// Wait for the command to finish
	errs := []error{}
	if err := buildCmd.Wait(); err != nil && err.(*exec.ExitError) == nil {
		errs = append(errs, fmt.Errorf("failed to execute build command: %w", err))
	}

	// Wait for log readers to finish
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
	err := errors.Join(errs...)

	return buildCmd.ProcessState.ExitCode(), err
}
