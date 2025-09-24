package build

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
)

type LogConsumer interface {
	CreateLog(ctx context.Context, log store.LogEntry) error
}

func build(
	bld *store.BuildWithRepoMeta,
	buildDir string,
	cmd []string,
	logs LogConsumer,
) (int, error) {
	buildDir, cacheDir, err := allocateBuildDir(buildDir, &bld.RepoMeta, bld.ID)
	if err != nil {
		return 0, err
	}
	if cacheDir != nil {
		if err := os.CopyFS(buildDir, os.DirFS(*cacheDir)); err != nil {
			return 0, fmt.Errorf(
				"failed to copy repo cache dir '%s' to build dir '%s'",
				*cacheDir, buildDir,
			)
		}
	}

	if err := checkout(&bld.RepoMeta, bld.CommitSHA, buildDir); err != nil {
		return 0, err
	}
	exitCode, err := runBuildCommand(logs, bld.ID, buildDir, cmd)
	if err != nil {
		return 0, err
	}

	// TODO: use repo to determine default branch - also handle ref vs branch
	err = freeBuildDir(buildDir, &bld.RepoMeta, bld.ID, bld.Ref == "master" || bld.Ref == "main")
	if err != nil {
		return 0, err
	}

	return exitCode, nil
}

func runBuildCommand(
	logConsumer LogConsumer,
	buildID uint64,
	buildDir string,
	cmd []string,
) (int, error) {
	buildCmd := exec.Command(cmd[0], cmd[1:]...)
	buildCmd.Dir = buildDir

	reader, writer := io.Pipe()

	// Combine stdout and stderr
	buildCmd.Stdout = writer
	buildCmd.Stderr = writer

	if err := buildCmd.Start(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			return 0, fmt.Errorf("failed to start build command: %w", err)
		}
	}

	errChan := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)

	// Read from the pipe and store as logs
	go func() {
		defer wg.Done()
		defer reader.Close()

		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			line := scanner.Text()
			log := store.LogEntry{
				BuildID:   buildID,
				Timestamp: time.Now(),
				Text:      line,
			}
			logConsumer.CreateLog(context.TODO(), log)
		}

		if err := scanner.Err(); err != nil {
			errChan <- fmt.Errorf("failed to execute build command: %w", err)
		} else {
			errChan <- nil
		}
	}()

	// Wait for the command to finish
	go func() {
		defer wg.Done()
		defer writer.Close()

		if err := buildCmd.Wait(); err != nil && err.(*exec.ExitError) == nil {
			errChan <- fmt.Errorf("failed to execute build command: %w", err)
		} else {
			errChan <- nil
		}
	}()

	wg.Wait()

	err := errors.Join(<-errChan, <-errChan)
	return buildCmd.ProcessState.ExitCode(), err
}
