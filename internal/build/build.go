package build

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"time"
)

const DEFAULT_BRANCH_CACHE = "default"
const DEFAULT_PERMS = 0700

type BuildState int

const (
	StateSuccess BuildState = iota
	StateFailure
	StateTimeout
)

type Builder struct {
	rootDir string
}

func NewBuilder(rootDir string) Builder {
	return Builder{
		rootDir: rootDir,
	}
}

func debugCmd(cmd *exec.Cmd) {
	fmt.Println("Running command:", cmd.Args)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}

func (b *Builder) prepareBuildDir(owner, name, branch, commitSHA string) (string, string, error) {
	// Create a dedicated build dir
	repoDir := fmt.Sprintf("%s:%s", owner, name)
	buildDir := path.Join(b.rootDir, repoDir, commitSHA)
	if err := os.MkdirAll(buildDir, DEFAULT_PERMS); err != nil {
		return "", "", fmt.Errorf("failed to obtain repo build dir '%s': %w", buildDir, err)
	}

	// Copy the contents of the default branch cache
	cacheDir := path.Join(b.rootDir, repoDir, DEFAULT_BRANCH_CACHE)
	fi, err := os.Stat(cacheDir)
	if os.IsNotExist(err) {
		// Nothing to copy
		return buildDir, cacheDir, nil
	}
	if err != nil {
		return "", "", fmt.Errorf("failed to obtain repo cache dir '%s': %w", cacheDir, err)
	}
	if !fi.Mode().IsDir() {
		return "", "", fmt.Errorf("repo cache path '%s' is not a directory", cacheDir)
	}

	if err := os.CopyFS(buildDir, os.DirFS(cacheDir)); err != nil {
		return "", "", fmt.Errorf("failed to copy repo cache dir '%s' to build dir '%s'", cacheDir, buildDir)
	}

	return buildDir, cacheDir, nil
}

func checkout(owner, name, commitSHA, buildDir string) error {
	initCmd := exec.Command("git", "-C", buildDir, "init", "-q")
	debugCmd(initCmd)
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to init repo at '%s': %v", buildDir, err)
	}

	cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
	cloneCmd := exec.Command("git", "-C", buildDir, "fetch", "--depth=1", cloneURL, commitSHA)
	debugCmd(cloneCmd)
	if err := cloneCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch repo at '%s': %v", cloneURL, err)
	}

	// Checkout repo files to build dir
	checkoutCmd := exec.Command(
		"git",
		"--git-dir", fmt.Sprintf("%s/.git", buildDir),
		"--work-tree", buildDir,
		"checkout", commitSHA, "--", ".",
	)
	debugCmd(checkoutCmd)
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit for '%s': %v", cloneURL, err)
	}

	return nil
}

type BuildResult struct {
	Duration time.Duration
	Success  bool
}

func build(buildDir string, cmd []string) (*BuildResult, error) {
	buildCmd := exec.Command(cmd[0], cmd[1:]...)
	buildCmd.Dir = buildDir
	debugCmd(buildCmd)

	start := time.Now()
	if err := buildCmd.Run(); err != nil && err.(*exec.ExitError) == nil {
		return nil, fmt.Errorf("failed to build repository: %w", err)
	}

	return &BuildResult{
		Duration: time.Since(start),
		Success:  buildCmd.ProcessState.ExitCode() == 0,
	}, nil
}

func (b *Builder) cleanup(branch, buildDir, cacheDir string) error {
	// TODO: use repo to determine default branch
	if branch == "master" || branch == "main" {
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

func (b *Builder) Build(owner, name, branch, commitSHA string, cmd []string) (*BuildResult, error) {
	buildDir, cacheDir, err := b.prepareBuildDir(owner, name, branch, commitSHA)
	if err != nil {
		return nil, err
	}
	if err := checkout(owner, name, commitSHA, buildDir); err != nil {
		return nil, err
	}
	result, err := build(buildDir, cmd)
	if err != nil {
		return nil, err
	}
	if err := b.cleanup(branch, buildDir, cacheDir); err != nil {
		return nil, err
	}
	return result, nil
}
