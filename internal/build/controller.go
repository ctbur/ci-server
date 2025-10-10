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
	DataDir string
}

type BuilderParams struct {
	DataDir   string
	Repo      config.RepoConfig
	Build     store.PendingBuild
	RunDeploy bool
}

// Create a new builder process by starting the same executable as the current
// process, but with the "builder" argument.
func (c *BuilderController) Start(
	repo config.RepoConfig, build store.PendingBuild, runDeploy bool,
) (int, error) {
	paramsJSON, err := json.Marshal(&BuilderParams{
		DataDir:   c.DataDir,
		Repo:      repo,
		Build:     build,
		RunDeploy: runDeploy,
	})
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

	logFile := getBuilderLogFile(c.DataDir, build.ID)
	// sec: Path is from a trusted user
	outFile, _ := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600) // #nosec G304
	builderCmd.Stdout = outFile
	builderCmd.Stderr = outFile

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
