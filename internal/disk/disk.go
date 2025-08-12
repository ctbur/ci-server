package disk

import (
	"os"
	"path"
)

const SUBDIR_REPOS = "repos"
const SUBDIR_BUILDS = "builds"
const DEFAULT_PERMS = 0700

type Disk struct {
	rootDir string
}

func New(rootDir string) Disk {
	return Disk{rootDir}
}

func (d *Disk) GetRepoDir(owner, name string) (string, error) {
	repoPath := path.Join(d.rootDir, SUBDIR_REPOS, owner, name)

	if err := os.MkdirAll(repoPath, DEFAULT_PERMS); err != nil {
		return "", err
	}

	return repoPath, nil
}

func (d *Disk) GetBuildDir(owner, name string) (string, error) {
	buildPath := path.Join(d.rootDir, SUBDIR_BUILDS, owner, name)

	if err := os.MkdirAll(buildPath, DEFAULT_PERMS); err != nil {
		return "", err
	}

	return buildPath, nil
}
