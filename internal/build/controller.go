package build

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type BuilderController struct {
	FS *store.FSStore
}

type BuilderParams struct {
	DataDir             string
	BuildID             uint64
	CacheID             *uint64
	RepoOwner, RepoName string
	CommitSHA           string
	PathEnvVar          string
	EnvVars             map[string]string
	BuildCmd            []string
	BuildSecrets        map[string]string
	DeployCmd           []string
	DeploySecrets       map[string]string
}

// Create a new builder process by starting the same executable as the current
// process, but with the "builder" argument.
func (c *BuilderController) Start(
	repo config.RepoConfig, build store.PendingBuild, runDeploy bool,
) (int, error) {

	params := BuilderParams{
		DataDir:      c.FS.RootDir,
		BuildID:      build.ID,
		CacheID:      build.CacheID,
		RepoOwner:    repo.Owner,
		RepoName:     repo.Name,
		CommitSHA:    build.CommitSHA,
		PathEnvVar:   os.Getenv("PATH"),
		EnvVars:      repo.EnvVars,
		BuildCmd:     repo.BuildCmd,
		BuildSecrets: repo.BuildSecrets,
	}

	if runDeploy {
		params.DeployCmd = repo.DeployCmd
		params.DeploySecrets = repo.DeploySecrets
	}

	paramsJSON, err := json.Marshal(&params)
	if err != nil {
		return 0, fmt.Errorf("failed to build params to JSON: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return 0, fmt.Errorf("failed to get executable path: %w", err)
	}

	// sec: exe is not user defined
	builderCmd := exec.Command(exe, "builder") // #nosec G204
	builderCmd.Env = []string{
		// Pass along PATH variable
		fmt.Sprintf("PATH=%s", os.Getenv("PATH")),
		// Add builder params to env var
		fmt.Sprintf("CI_BUILDER_PARAMS=%s", paramsJSON),
		// Set build ID separately as it helps with identification of the builder process
		fmt.Sprintf("CI_BUILDER_BUILD_ID=%d", build.ID),
	}

	// Keep builder running independently of server
	builderCmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	logWriter, err := c.FS.OpenBuilderLogs(build.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to open builder logs: %w", err)
	}
	builderCmd.Stdout = logWriter
	builderCmd.Stderr = logWriter

	if err := builderCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start builder process: %w", err)
	}

	return builderCmd.Process.Pid, nil
}

// isBuilderRunning checks if a builder process is still running. The process is
// identified using PID and build ID in env. This is to protect against PID
// reuse.
func (c *BuilderController) IsRunning(pid int, buildID uint64) bool {
	// Check if the process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Check if process is running by sending signal 0 - necessary on Unix
	err = process.Signal(syscall.Signal(0))
	if err != nil {
		return false
	}

	// Check if the BUILD_ID environment variable matches
	envPath := fmt.Sprintf("/proc/%d/environ", pid)
	// sec: Path is restricted
	data, err := os.ReadFile(envPath) // #nosec G304
	if err != nil {
		return false
	}

	envVars := strings.Split(string(data), "\000")
	buildEnvVar := fmt.Sprintf("CI_BUILDER_BUILD_ID=%d", buildID)
	if !slices.Contains(envVars, buildEnvVar) {
		return false
	}

	return true
}
