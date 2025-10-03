package build

import (
	"fmt"
	"path"
	"strconv"
)

/*
 * Directory structure:
 * rootDir/
 *   build/
 *     <ID>/             build dir for build with ID
 *	     files/          actual build files
 *       builder.json    metadata of the builder
 *       exit_code       exit code of the build command
 *   cache/
 *     <owner>/
 *       <repo>/
 *         <ID>/         cache for default branch
 *   logs/
 *     <ID>.jsonl        log file for build with ID
 *
 *
 * The cache dir is used to speed up builds on the default branch.
 * It is copied into the build dir before the build starts.
 * After a successful build on the default branch, the build dir
 * is moved to the cache dir.
 */

func getBuilderFile(dataDir string, buildID uint64) string {
	return path.Join(getBuildDir(dataDir, buildID), "builder.json")
}

func getLogFile(dataDir string, buildID uint64) string {
	return path.Join(dataDir, "logs", fmt.Sprintf("%d.jsonl", buildID))
}

func getBuildFilesDir(dataDir string, buildID uint64) string {
	return path.Join(getBuildDir(dataDir, buildID), "files")
}

func getExitCodeFile(dataDir string, buildID uint64) string {
	return path.Join(getBuildDir(dataDir, buildID), "exit_code")
}

func getBuildDir(dataDir string, buildID uint64) string {
	return path.Join(getBuildRoot(dataDir), strconv.FormatUint(buildID, 10))
}

func getBuildRoot(dataDir string) string {
	return path.Join(dataDir, "build")
}

func getCacheDir(dataDir string, repoOwner, repoName string, cacheID uint64) string {
	return path.Join(getCacheRootDir(dataDir, repoOwner, repoName), strconv.FormatUint(cacheID, 10))
}

func getCacheRootDir(dataDir string, repoOwner, repoName string) string {
	return path.Join(dataDir, "cache", repoOwner, repoName)
}
