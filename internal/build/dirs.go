package build

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/ctbur/ci-server/v2/internal/store"
)

const defaultBranchCache = "default"
const defaultPerms = 0700

func getBuildAndCacheDirs(rootDir string, repo *store.RepoMeta, buildID uint64) (string, string) {
	repoDir := fmt.Sprintf("%s/%s", repo.Owner, repo.Name)
	buildDir := path.Join(rootDir, repoDir, strconv.FormatUint(buildID, 10))
	cacheDir := path.Join(rootDir, repoDir, defaultBranchCache)
	return buildDir, cacheDir
}

func allocateBuildDir(rootDir string, repo *store.RepoMeta, buildID uint64) (string, *string, error) {
	buildDir, cacheDir := getBuildAndCacheDirs(rootDir, repo, buildID)

	// Create a dedicated build dir
	if err := os.MkdirAll(buildDir, defaultPerms); err != nil {
		return "", nil, fmt.Errorf("failed to obtain repo build dir '%s': %w", buildDir, err)
	}

	// Copy the contents of the default branch cache
	fi, err := os.Stat(cacheDir)
	if os.IsNotExist(err) {
		// Nothing to copy
		return buildDir, nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("failed to obtain repo cache dir '%s': %w", cacheDir, err)
	}
	if !fi.Mode().IsDir() {
		return "", nil, fmt.Errorf("repo cache path '%s' is not a directory", cacheDir)
	}

	return buildDir, &cacheDir, nil
}

func freeBuildDir(rootDir string, repo *store.RepoMeta, buildID uint64, makeCache bool) error {
	buildDir, cacheDir := getBuildAndCacheDirs(rootDir, repo, buildID)

	if makeCache {
		// Cache the build results
		// TODO: figure out how to avoid concurrency issues when other build is copying cache dir
		if err := os.RemoveAll(cacheDir); err != nil {
			return fmt.Errorf("failed to clean up cache dir '%s': %w", cacheDir, err)
		}
		if err := os.Rename(buildDir, cacheDir); err != nil {
			return fmt.Errorf("failed to turn build dir '%s' into cache dir '%s': %w", buildDir, cacheDir, err)
		}
	} else {
		if err := os.RemoveAll(buildDir); err != nil {
			return fmt.Errorf("failed to clean up build dir '%s': %w", buildDir, err)
		}
	}

	return nil
}
