package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
)

type DataDir struct {
	RootDir            string
	modifiedRepoCaches map[string]struct{}
}

func (d *DataDir) ListBuilders() ([]Builder, error) {
	buildIDs, err := readDirIDs(getBuildRoot(d.RootDir))
	if err != nil {
		return nil, err
	}

	var builders []Builder
	var errs []error
	for _, buildID := range buildIDs {
		builderFile := getBuilderFile(d.RootDir, buildID)

		data, err := os.ReadFile(builderFile)
		if err != nil {
			errs = append(errs,
				fmt.Errorf("failed to read builder file for ID %d: %w", buildID, err),
			)
			continue
		}

		var builder Builder
		if err := json.Unmarshal(data, &builder); err != nil {
			errs = append(errs,
				fmt.Errorf("failled to unmarshal builder from JSON for ID %d: %w", buildID, err),
			)
			continue
		}
	}

	return builders, errors.Join(errs...)
}

func (d *DataDir) GetExitCode(buildID uint64) (int, error) {
	data, err := os.ReadFile(getExitCodeFile(d.RootDir, buildID))
	if err != nil {
		return 0, err
	}

	exitCode, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return 0, err
	}

	return int(exitCode), nil
}

func (d *DataDir) RemoveBuilder(b Builder, cacheBuildFiles bool) error {
	if cacheBuildFiles {
		cacheRootDir := getCacheRootDir(d.RootDir, b.RepoOwner, b.RepoName)
		err := os.MkdirAll(cacheRootDir, 0o700)
		if err != nil {
			return fmt.Errorf("failed to create cache dir for repo: %w", err)
		}

		cacheDir := getCacheDir(d.RootDir, b.RepoOwner, b.RepoName, b.BuildID)
		buildFilesDir := getBuildFilesDir(d.RootDir, b.BuildID)
		err = os.Rename(buildFilesDir, cacheDir)
		if err != nil {
			return fmt.Errorf("failed to move build files to cache: %w", err)
		}

		d.modifiedRepoCaches[fmt.Sprintf("%s/%s", b.RepoOwner, b.RepoName)] = struct{}{}
	}

	buildDir := getBuildDir(d.RootDir, b.BuildID)
	if err := os.RemoveAll(buildDir); err != nil {
		return fmt.Errorf("failed to delete build dir: %w", err)
	}
	return nil
}

func (d *DataDir) GetLatestCache(repoOwner, repoName string) (*uint64, error) {
	// List all directories in cacheRootDir and find the one with the highest build ID
	entries, err := os.ReadDir(getCacheRootDir(d.RootDir, repoOwner, repoName))
	if os.IsNotExist(err) {
		// No cache dir exists yet
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read cache root dir: %w", err)
	}

	var latestBuildID *uint64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		buildID, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err == nil {
			if latestBuildID == nil {
				latestBuildID = &buildID
			} else if buildID > *latestBuildID {
				latestBuildID = &buildID
			}
		}
	}

	return latestBuildID, nil
}

func readDirIDs(dir string) ([]uint64, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var ids []uint64
	for _, entry := range entries {
		id, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil {
			continue
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func (d *DataDir) AddBuilder(builder Builder) error {
	buildDir := getBuildDir(d.RootDir, builder.BuildID)

	if err := os.MkdirAll(buildDir, 0o700); err != nil {
		return fmt.Errorf("failed to create build dir: %w", err)
	}

	builderFile := getBuilderFile(d.RootDir, builder.BuildID)
	data, err := json.Marshal(&builder)
	if err != nil {
		return fmt.Errorf("failed to marshal builder: %w", err)
	}

	if err := os.WriteFile(builderFile, data, 0o644); err != nil {
		fmt.Errorf("failed to write builder file: %w", err)
	}
}
