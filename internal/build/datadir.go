package build

import (
	"errors"
	"fmt"
	"os"
	"path"
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
	data, err := os.ReadFile(exitCodeFile)
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

func (d *DataDir) RetainBuildDirs(retainedIDs []uint64) error {
	buildRootDir := path.Join(d.RootDir, "build")
	entries, err := os.ReadDir(buildRootDir)
	if err != nil {
		return err
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
	for _, id := range existingIDs {
		// delete if not in use
		if slices.Contains(retainedIDs, id) {
			continue
		}

		buildDir := path.Join(buildRootDir, strconv.FormatUint(id, 10))
		err := os.RemoveAll(buildDir)
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to delete cache dir: %w", err))
		}
	}

	return errors.Join(errs...)
}

func getLogFile(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "logs", fmt.Sprintf("%d.jsonl", buildID))
}

func getExitCodeFile(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "exit_code", strconv.FormatUint(buildID, 10))
}

func getBuildDir(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "build", strconv.FormatUint(buildID, 10))
}
