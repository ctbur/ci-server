package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path"
)

type Builder struct {
	Dir              builderDataDir
	Git              git
	RepoURLFormatter func(owner, name string) string
	Cmd              cmdRunner
}

type builderDataDir interface {
	CreateBuildDir(buildID uint64, cacheID *uint64, checkoutDir string) (string, error)
	WriteExitCode(buildID uint64, exitCode int) error
}

type git interface {
	Checkout(repoURL, commitSHA, targetDir string) error
}

type cmdRunner interface {
	Run(buildID uint64, absSandboxDir, workDir string, cmd []string, env []string) (int, error)
}

func githubRepoURL(owner, name string) string {
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, name)
}

func RunBuilder() error {
	paramsJSON := os.Getenv("CI_BUILDER_PARAMS")
	if paramsJSON == "" {
		return errors.New("missing CI_BUILDER_PARAMS for builder")
	}

	p := BuilderParams{}
	if err := json.Unmarshal([]byte(paramsJSON), &p); err != nil {
		return fmt.Errorf("failed to unmarshal build params JSON '%s': %w", paramsJSON, err)
	}

	dataDir := &DataDir{RootDir: p.DataDir}
	br := Builder{
		Dir:              dataDir,
		Git:              &Git{},
		RepoURLFormatter: githubRepoURL,
		Cmd:              &CmdRunner{dataDir},
	}

	return br.run(slog.Default(), p)
}

func (br *Builder) run(log *slog.Logger, p BuilderParams) error {
	exitCode, err := br.runBuild(log, p)
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	if err := br.Dir.WriteExitCode(p.BuildID, exitCode); err != nil {
		return fmt.Errorf("failed to write exit code: %w", err)
	}
	slog.Info("Wrote exit code", slog.Int("exit_code", exitCode))

	return nil
}

func (br *Builder) runBuild(log *slog.Logger, p BuilderParams) (int, error) {
	// Prepare build dir
	if p.CacheID != nil {
		log.Info("Copying cache", slog.Uint64("cache_id", *p.CacheID))
	} else {
		log.Info("Starting from an empty build dir")
	}
	checkoutDir := path.Join(p.RepoOwner, p.RepoName)
	absBuildDir, err := br.Dir.CreateBuildDir(
		p.BuildID, p.CacheID,
		checkoutDir,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to create build dir: %w", err)
	}

	// Checkout
	absCheckoutDir := path.Join(absBuildDir, checkoutDir)
	repoURL := br.RepoURLFormatter(p.RepoOwner, p.RepoName)
	err = br.Git.Checkout(repoURL, p.CommitSHA, absCheckoutDir)
	if err != nil {
		return 0, err
	}

	// Run build command
	log.Info("Starting build...", slog.Any("command", p.BuildCmd))
	buildEnv := buildCmdEnv(absBuildDir, p.PathEnvVar, p.EnvVars, p.BuildSecrets)
	exitCode, err := br.Cmd.Run(p.BuildID, absBuildDir, absCheckoutDir, p.BuildCmd, buildEnv)
	if err != nil {
		return 0, err
	}
	log.Info("Finished build command", slog.Int("exit_code", exitCode))

	// Don't run deploy if there is no command
	if len(p.DeployCmd) == 0 {
		slog.Info("No deploy command provided")
		return 0, nil
	}
	// Don't run deploy if build failed
	if exitCode != 0 {
		slog.Info("Deploy command provided, but build command failed")
		return exitCode, nil
	}

	// Run deploy command
	log.Info("Starting deploy...", slog.Any("command", p.DeployCmd))
	deployEnv := buildCmdEnv(absBuildDir, p.PathEnvVar, p.EnvVars, p.DeploySecrets)
	exitCode, err = br.Cmd.Run(p.BuildID, absBuildDir, absCheckoutDir, p.DeployCmd, deployEnv)
	if err != nil {
		return 0, err
	}
	log.Info("Finished deploy command", slog.Int("exit_code", exitCode))

	return exitCode, nil
}

func buildCmdEnv(absBuildDir string, pathEnvVar string, envVars map[string]string, secrets map[string]string) []string {
	var env []string

	// Add default env vars
	env = append(env,
		"CI=true",
		// Pass along PATH variable
		fmt.Sprintf("PATH=%s", pathEnvVar),
		// Set build dir as HOME
		fmt.Sprintf("HOME=%s", absBuildDir),
	)

	for secret, value := range secrets {
		env = append(env, fmt.Sprintf("%s=%s", secret, value))
	}
	for name, value := range envVars {
		env = append(env, fmt.Sprintf("%s=%s", name, value))
	}

	return env
}

type Git struct{}

func (g *Git) Checkout(repoURL, commitSHA, targetDir string) error {
	initCmd := exec.Command("git", "-C", targetDir, "init", "-q")
	if err := initCmd.Run(); err != nil {
		return fmt.Errorf("failed to init repo at '%s': %w", targetDir, err)
	}

	// sec: Path comes from a trusted user, other args are not security critical
	fetchCmd := exec.Command("git", "-C", targetDir, "fetch", "--depth=1", repoURL, commitSHA) // #nosec G204
	if err := fetchCmd.Run(); err != nil {
		return fmt.Errorf("failed to fetch repo at '%s': %w", repoURL, err)
	}

	// Check out repo files to build dir
	// sec: Path comes from a trusted user, other args are not security critical
	checkoutCmd := exec.Command(
		"git",
		"--git-dir", fmt.Sprintf("%s/.git", targetDir),
		"--work-tree", targetDir,
		"checkout", "-f", commitSHA,
	) // #nosec G204
	if err := checkoutCmd.Run(); err != nil {
		return fmt.Errorf("failed to checkout commit for '%s': %w", repoURL, err)
	}

	return nil
}
