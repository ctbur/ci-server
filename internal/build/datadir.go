package build

import (
	"errors"
	"fmt"
	"os"
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
 *   logs/
 *     <ID>.jsonl        log file for build with ID
 *
 *
 * The cache dir is used to speed up builds on the default branch.
 * It is copied into the build dir before the build starts.
 * After a successful build on the default branch, the build dir
 * is moved to the cache dir.
 */

type DataDir struct {
	RootDir            string
	modifiedRepoCaches map[string]struct{}
}

func (d *DataDir) ReadAndCleanExitCode(buildID uint64) (int, error) {
	exitCodeFile := getExitCodeFile(d.RootDir, buildID)
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

func (d *DataDir) RetainBuildDirs(retainedIDs []uint64) ([]uint64, error) {
	buildRootDir := path.Join(d.RootDir, "build")
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
			_ = os.Chmod(name, 0600)
		}
		return nil
	})

	// Try again
	return os.RemoveAll(path)
}

func (d *DataDir) CreateRootDirs() error {
	if err := os.MkdirAll(path.Join(d.RootDir, "build-logs"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(d.RootDir, "builder-logs"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(d.RootDir, "exit-code"), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(path.Join(d.RootDir, "build"), 0o700); err != nil {
		return err
	}
	return nil
}

func getLogFile(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "build-logs", fmt.Sprintf("%d.jsonl", buildID))
}

func getBuilderLogFile(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "builder-logs", fmt.Sprintf("%d.txt", buildID))
}

func getExitCodeFile(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "exit-code", strconv.FormatUint(buildID, 10))
}

func getBuildDir(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "build", strconv.FormatUint(buildID, 10))
}
