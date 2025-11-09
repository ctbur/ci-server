package build

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
)

type CmdRunner struct {
	FS *store.FSStore
}

func (r *CmdRunner) Run(
	buildID uint64,
	absSandboxDir, workDir string,
	cmd []string,
	env []string,
) (int, error) {
	// Run command in bubblewrap sandbox
	var bwrapSandbox = []string{
		"bwrap",
		"--die-with-parent",
		"--unshare-all", "--share-net",
		"--ro-bind", "/", "/",
		"--dev", "/dev",
		"--tmpfs", "/tmp",
		"--bind", absSandboxDir, absSandboxDir,
		// Run all commands in the dir where we check out the repo
		"--chdir", workDir,
	}
	cmd = append(bwrapSandbox, cmd...)
	// sec: Command is from a trusted user
	execCmd := exec.Command(cmd[0], cmd[1:]...) // #nosec G204
	execCmd.Env = env

	logWriter, err := r.FS.OpenBuildLogs(buildID)
	if err != nil {
		return 0, fmt.Errorf("failed to open build logs: %w", err)
	}
	defer logWriter.Close()

	return runAndLog(execCmd, logWriter)
}

func runAndLog(cmd *exec.Cmd, logWriter io.Writer) (int, error) {
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

		encoder := json.NewEncoder(logWriter)
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
