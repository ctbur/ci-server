package store

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
)

/*
 * Directory structure:
 * rootDir/
 *   build/
 *     <ID>/             build dir for build with ID
 *   exit_code/
 *     <ID>            exit code of the build command
 *   build-logs/
 *     <ID>.jsonl        log file for build with ID
 *   builder-logs/
 *     <ID>.jsonl        log file for builder with ID
 *
 *
 * The cache dir is used to speed up builds on the default branch.
 * It is copied into the build dir before the build starts.
 * After a successful build on the default branch, the build dir
 * is moved to the cache dir.
 */

type FSStore struct {
	RootDir string
}

func (f *FSStore) WriteExitCode(buildID uint64, exitCode int) error {
	exitCodePath := path.Join(f.RootDir, "exit-code", strconv.FormatUint(buildID, 10))
	return os.WriteFile(exitCodePath, []byte(strconv.Itoa(exitCode)), 0o600)
}

func (f *FSStore) ReadAndCleanExitCode(buildID uint64) (int, error) {
	exitCodeFile := path.Join(f.RootDir, "exit-code", strconv.FormatUint(buildID, 10))
	// sec: Path is from a trusted user
	data, err := os.ReadFile(exitCodeFile) // #nosec G304
	if err != nil {
		return 0, err
	}

	exitCode, err := strconv.ParseUint(string(data), 10, 32)
	if err != nil {
		return 0, err
	}

	if err := os.Remove(exitCodeFile); err != nil {
		return int(exitCode), err
	}

	return int(exitCode), nil
}

// CreateBuildDir creates directory, which contains another directory under
// checkoutDir. If the cacheID is given, files from the build dir with the same
// ID are copied into the directory beforehand.
// It returns the absolute build dir path, or the first error encountered.
func (f *FSStore) CreateBuildDir(
	buildID uint64, cacheID *uint64, checkoutDir string,
) (string, error) {
	buildDir := path.Join(f.RootDir, "build", strconv.FormatUint(buildID, 10))

	if cacheID != nil {
		cacheDir := path.Join(f.RootDir, "build", strconv.FormatUint(*cacheID, 10))
		// copyDirs will create the build dir
		if err := copyDirs(cacheDir, buildDir); err != nil {
			return "", fmt.Errorf(
				"failed to copy repo cache dir '%s' to build dir '%s': %w",
				cacheDir, buildDir, err,
			)
		}
	} else {
		if err := os.Mkdir(buildDir, 0o700); err != nil {
			return "", fmt.Errorf("failed to create empty dir: %w", err)
		}
	}

	// Ensure checkout dir exists
	if err := os.MkdirAll(path.Join(buildDir, checkoutDir), 0o700); err != nil {
		return "", fmt.Errorf("failed to create checkout dir: %w", err)
	}

	// Get absolute build dir path for sandbox parameters
	absBuildDir, err := filepath.Abs(buildDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve build dir path: %w", err)
	}

	return absBuildDir, nil
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

func (f *FSStore) RetainBuildDirs(retainedIDs []uint64) ([]uint64, error) {
	buildRootDir := path.Join(f.RootDir, "build")
	entries, err := os.ReadDir(buildRootDir)
	if err != nil {
		return nil, err
	}

	var existingIDs []uint64
	for _, entry := range entries {
		id, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil {
			continue
		}

		existingIDs = append(existingIDs, id)
	}

	var errs []error
	var deletedIDs []uint64
	for _, id := range existingIDs {
		// delete if not in use
		if slices.Contains(retainedIDs, id) {
			continue
		}

		buildDir := path.Join(buildRootDir, strconv.FormatUint(id, 10))
		err := removeAll(buildDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete cache dir: %w", err))
			continue
		}

		deletedIDs = append(deletedIDs, id)
	}

	return deletedIDs, errors.Join(errs...)
}

func removeAll(path string) error {
	// Try RemoveAll directly
	err := os.RemoveAll(path)
	if err == nil {
		return nil
	}

	// Try to give write permissions to every file
	_ = filepath.Walk(path, func(name string, info os.FileInfo, err error) error {
		if err == nil {
			// sec: 0700 is required to ensure deletion
			_ = os.Chmod(name, 0700) // #nosec G302
		}
		return nil
	})

	// Try again
	return os.RemoveAll(path)
}

func (f *FSStore) CreateRootDirs() error {
	if err := os.MkdirAll(path.Join(f.RootDir, "build-logs"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(f.RootDir, "builder-logs"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(f.RootDir, "exit-code"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(f.RootDir, "build"), 0o700); err != nil {
		return err
	}
	return nil
}

func (f *FSStore) OpenBuildLogs(buildID uint64) (io.WriteCloser, error) {
	logFilePath := path.Join(f.RootDir, "build-logs", fmt.Sprintf("%d.jsonl", buildID))
	// sec: Path is from a trusted user
	return os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304
}

func (f *FSStore) OpenBuilderLogs(buildID uint64) (io.WriteCloser, error) {
	logFilePath := path.Join(f.RootDir, "builder-logs", fmt.Sprintf("%d.txt", buildID))
	// sec: Path is from a trusted user
	return os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600) // #nosec G304
}
