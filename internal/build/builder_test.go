package build

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/test"
)

func writeToDir(targetDir string, files map[string]string) error {
	for name, content := range files {
		filePath := path.Join(targetDir, name)
		if err := os.WriteFile(filePath, []byte(content), 0o600); err != nil {
			return fmt.Errorf("failed to write file '%s': %w", filePath, err)
		}
	}
	return nil
}

func createDummyGitRepo(targetDir string, files map[string]string) (string, error) {
	// Create repo
	initCmd := exec.Command("git", "-C", targetDir, "init")
	if err := initCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to init repo at '%s': %w", targetDir, err)
	}

	// Create dummy files
	writeToDir(targetDir, files)

	// Commit all files
	addCmd := exec.Command("git", "-C", targetDir, "add", "--all")
	if err := addCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to add files: %w", err)
	}
	commitCmd := exec.Command("git", "-C", targetDir, "commit", "-am", "Test commit")
	if err := commitCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to commit files: %w", err)
	}

	// Read and return SHA
	shaCmd := exec.Command("git", "-C", targetDir, "rev-parse", "HEAD")
	var shaBytes bytes.Buffer
	shaCmd.Stdout = &shaBytes

	if err := shaCmd.Run(); err != nil {
		return "", fmt.Errorf("failed to read HEAD SHA: %w", err)
	}

	return strings.TrimSpace(shaBytes.String()), nil
}

func (r *MockCmdRunner) Run(
	buildID uint64,
	absSandboxDir, workDir string,
	cmd []string,
	env []string,
) (int, error) {
	r.Calls = append(r.Calls, CmdRunnerCall{
		buildID, absSandboxDir, workDir, cmd, env,
	})
	res := r.MockResults[0]
	r.MockResults = r.MockResults[1:]
	return res.exitCode, res.err
}

func TestBuilderE2E(t *testing.T) {
	if os.Getenv("CI") != "" {
		// TODO: find way to run nested sandbox
		t.Skip("Skipping test on CI because sandbox cannot run within itself")
	}

	testDir, err := os.MkdirTemp("", "builder-e2e")
	assert.NoError(t, err, "Failed to create temp dir").Fatal()
	defer os.RemoveAll(testDir)

	// Init DataDir
	dataDirPath := path.Join(testDir, "data-dir")
	dataDir := store.FSStore{
		RootDir: dataDirPath,
	}
	err = dataDir.CreateRootDirs()
	assert.NoError(t, err, "Failed to init DataDir")

	buildID := uint64(21)
	cacheID := uint64(11)

	// Create cache
	cacheDir, err := dataDir.CreateBuildDir(cacheID, nil, "owner/repo")
	assert.NoError(t, err, "Failed to create cache dir").Fatal()
	err = writeToDir(path.Join(cacheDir, "owner/repo"), map[string]string{
		"A": "cached file",
		"B": "some other cached file",
	})
	assert.NoError(t, err, "Failed to init cache dir").Fatal()

	// Create Git repo that is going to be cloned
	repoDir := path.Join(testDir, "git-repo")
	err = os.Mkdir(repoDir, 0o700)
	assert.NoError(t, err, "Failed to create repo dir").Fatal()

	dummyCommitSHA, err := createDummyGitRepo(repoDir, map[string]string{
		"A": "committed file",
		"C": "some other committed file",
	})
	assert.NoError(t, err, "Failed to create dummy Git repo").Fatal()

	// Run build
	p := BuilderParams{
		DataDir:    dataDirPath,
		BuildID:    buildID,
		CacheID:    &cacheID,
		RepoOwner:  "owner",
		RepoName:   "repo",
		CommitSHA:  dummyCommitSHA,
		PathEnvVar: os.Getenv("PATH"),
		EnvVars: map[string]string{
			"ENV_VAR_A": "env A",
			"ENV_VAR_B": "env B",
		},
		BuildCmd: []string{
			"sh", "-c", "printenv > build.env",
		},
		BuildSecrets: map[string]string{
			"BUILD_SECRET_A": "build A",
			"BUILD_SECRET_B": "build B",
		},
		DeployCmd: []string{
			"sh", "-c", "printenv > deploy.env",
		},
		DeploySecrets: map[string]string{
			"DEPLOY_SECRET_A": "deploy A",
			"DEPLOY_SECRET_B": "deploy B",
		},
	}

	br := Builder{
		FS:  &dataDir,
		Git: &Git{},
		RepoURLFormatter: func(owner, name string) string {
			return fmt.Sprintf("file://%s", repoDir)
		},
		Cmd: &CmdRunner{FS: &dataDir},
	}

	log := test.Logger(t)
	err = br.run(log, p)
	assert.NoError(t, err, "Failed to run builder").Fatal()

	// Check exit code correct
	exitCode, err := dataDir.ReadAndCleanExitCode(buildID)
	assert.NoError(t, err, "Failed to read exit code")
	assert.Equal(t, exitCode, 0, "Incorrect exit code written")

	// Check files in build dir are correct
	buildDir := path.Join(dataDirPath, "build", strconv.FormatUint(buildID, 10), "owner/repo")
	assert.FileContents(t, path.Join(buildDir, "A"), "committed file", "msg")
	assert.FileContents(t, path.Join(buildDir, "B"), "some other cached file", "msg")
	assert.FileContents(t, path.Join(buildDir, "C"), "some other committed file", "msg")

	// Check printenv contents
	buildEnv, err := os.ReadFile(path.Join(buildDir, "build.env"))
	assert.NoError(t, err, "Failed to read build.env")
	checkEnv(
		t, buildEnv,
		[]string{
			"ENV_VAR_A=env A",
			"ENV_VAR_B=env B",
			"BUILD_SECRET_A=build A",
			"BUILD_SECRET_B=build B",
		},
		[]string{
			"DEPLOY_SECRET_A=deploy A",
			"DEPLOY_SECRET_B=deploy B",
		},
	)
	deployEnv, err := os.ReadFile(path.Join(buildDir, "deploy.env"))
	assert.NoError(t, err, "Failed to read deploy.env")
	checkEnv(
		t, deployEnv,
		[]string{
			"ENV_VAR_A=env A",
			"ENV_VAR_B=env B",
			"DEPLOY_SECRET_A=deploy A",
			"DEPLOY_SECRET_B=deploy B",
		},
		[]string{
			"BUILD_SECRET_A=build A",
			"BUILD_SECRET_B=build B",
		},
	)
}

func checkEnv(t *testing.T, envData []byte, contains []string, notContains []string) {
	t.Helper()

	env := strings.Split(strings.TrimSpace(string(envData)), "\n")

	for _, c := range contains {
		found := slices.Contains(env, c)
		assert.Equal(t, found, true, fmt.Sprintf("env does not contain %s", c))
	}

	for _, nc := range notContains {
		found := slices.Contains(env, nc)
		assert.Equal(t, found, false, fmt.Sprintf("env contains %s", nc))
	}
}

func NewMockDataDir() MockDataDir {
	return MockDataDir{
		BuildDirs: make(map[uint64]MockBuildDir),
		ExitCodes: make(map[uint64]int),
	}
}

type MockDataDir struct {
	BuildDirs map[uint64]MockBuildDir
	ExitCodes map[uint64]int
}

type MockBuildDir struct {
	CacheID     *uint64
	CheckoutDir string
}

func (d *MockDataDir) CreateBuildDir(buildID uint64, cacheID *uint64, checkoutDir string) (string, error) {
	if _, exists := d.BuildDirs[buildID]; exists {
		return "", errors.New("build dir exists")
	}

	d.BuildDirs[buildID] = MockBuildDir{CacheID: cacheID, CheckoutDir: checkoutDir}
	return fmt.Sprintf("/mockdir/%d", buildID), nil
}

func (d *MockDataDir) WriteExitCode(buildID uint64, exitCode int) error {
	if _, exists := d.ExitCodes[buildID]; exists {
		return errors.New("exit code already written")
	}

	d.ExitCodes[buildID] = exitCode
	return nil
}

type MockCmdRunner struct {
	MockResults []MockCmdResult
	Calls       []CmdRunnerCall
}

type MockCmdResult struct {
	exitCode int
	err      error
}

type CmdRunnerCall struct {
	buildID                uint64
	absSandboxDir, workDir string
	cmd                    []string
	env                    []string
}

type MockGit struct {
	RepoURL   string
	CommitSHA string
	TargetDir string
}

func (g *MockGit) Checkout(repoURL, commitSHA, targetDir string) error {
	g.RepoURL = repoURL
	g.CommitSHA = commitSHA
	g.TargetDir = targetDir
	return nil
}

func TestBuilder(t *testing.T) {
	cacheID := uint64(99)
	testCases := []struct {
		desc         string
		buildID      uint64
		cacheID      *uint64
		buildCmd     []string
		deployCmd    []string
		cmdResults   []MockCmdResult
		wantExitCode int
		shouldDeploy bool
	}{
		{
			desc:     "Build without cache",
			buildID:  101,
			cacheID:  nil,
			buildCmd: []string{"make", "lint", "test"},
			cmdResults: []MockCmdResult{
				{exitCode: 0, err: nil},
			},
			wantExitCode: 0,
			shouldDeploy: false,
		},
		{
			desc:     "Build with cache",
			buildID:  101,
			cacheID:  &cacheID,
			buildCmd: []string{"make", "lint", "test"},
			cmdResults: []MockCmdResult{
				{exitCode: 0, err: nil},
			},
			wantExitCode: 0,
			shouldDeploy: false,
		},
		{
			desc:     "Build fails",
			buildID:  101,
			cacheID:  &cacheID,
			buildCmd: []string{"make", "lint", "test"},
			cmdResults: []MockCmdResult{
				{exitCode: 5, err: nil},
			},
			wantExitCode: 5,
			shouldDeploy: false,
		},
		{
			desc:      "Build and deploy",
			buildID:   101,
			cacheID:   &cacheID,
			buildCmd:  []string{"make", "lint", "test"},
			deployCmd: []string{"make", "install"},
			cmdResults: []MockCmdResult{
				{exitCode: 0, err: nil},
				{exitCode: 0, err: nil},
			},
			wantExitCode: 0,
			shouldDeploy: true,
		},
		{
			desc:      "Build and deploy, but both commands fail",
			buildID:   101,
			cacheID:   &cacheID,
			buildCmd:  []string{"make", "lint", "test"},
			deployCmd: []string{"make", "install"},
			cmdResults: []MockCmdResult{
				{exitCode: 1, err: nil},
				{exitCode: 2, err: nil},
			},
			wantExitCode: 1,
			shouldDeploy: false,
		},
		{
			desc:      "Build and deploy, but only deploy fails",
			buildID:   101,
			cacheID:   &cacheID,
			buildCmd:  []string{"make", "lint", "test"},
			deployCmd: []string{"make", "install"},
			cmdResults: []MockCmdResult{
				{exitCode: 0, err: nil},
				{exitCode: 3, err: nil},
			},
			wantExitCode: 3,
			shouldDeploy: true,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			commitSHA := "3745d6557067287eda95a33b9b2e5bfc3f21171a"
			p := BuilderParams{
				DataDir:    "",
				BuildID:    tc.buildID,
				CacheID:    tc.cacheID,
				RepoOwner:  "owner",
				RepoName:   "repo",
				CommitSHA:  commitSHA,
				PathEnvVar: "/usr/lib/go/bin:/usr/local/bin:/usr/bin",
				EnvVars: map[string]string{
					"ENV_VAR_A": "env A",
					"ENV_VAR_B": "env B",
				},
				BuildCmd: tc.buildCmd,
				BuildSecrets: map[string]string{
					"BUILD_SECRET_A": "build A",
					"BUILD_SECRET_B": "build B",
				},
				DeployCmd: tc.deployCmd,
				DeploySecrets: map[string]string{
					"DEPLOY_SECRET_A": "deploy A",
					"DEPLOY_SECRET_B": "deploy B",
				},
			}

			// Run builder
			dataDir := NewMockDataDir()
			git := MockGit{}
			cmdRunner := MockCmdRunner{
				MockResults: tc.cmdResults,
			}
			br := Builder{
				FS:               &dataDir,
				Git:              &git,
				RepoURLFormatter: githubRepoURL,
				Cmd:              &cmdRunner,
			}

			log := test.Logger(t)
			err := br.run(log, p)
			assert.NoError(t, err, "Failed to run builder")

			// Check data dir
			assert.Equal(t, dataDir.BuildDirs[tc.buildID].CacheID, tc.cacheID, "Incorrect cache ID")
			assert.Equal(t, dataDir.BuildDirs[tc.buildID].CheckoutDir, "owner/repo", "Incorrect checkout dir")
			assert.Equal(t, dataDir.ExitCodes[tc.buildID], tc.wantExitCode, "Incorrect exit code")

			// Check git
			assert.Equal(t, git.RepoURL, "https://github.com/owner/repo.git", "Incorrect repo URL")
			assert.Equal(t, git.CommitSHA, commitSHA, "Incorrect commit SHA")
			wantCheckoutDir := fmt.Sprintf("/mockdir/%d/owner/repo", tc.buildID)
			assert.Equal(t, git.TargetDir, wantCheckoutDir, "Incorrect repo dir")

			// Check cmd runner
			if tc.shouldDeploy {
				assert.Equal(t, len(cmdRunner.Calls), 2, "Incorrect number of commands executed")
			} else {
				assert.Equal(t, len(cmdRunner.Calls), 1, "Incorrect number of commands executed")
			}

			// Check build cmd
			assert.Equal(t, cmdRunner.Calls[0].buildID, tc.buildID, "Incorrect build ID")
			wantSandboxDir := fmt.Sprintf("/mockdir/%d", tc.buildID)
			assert.Equal(t, cmdRunner.Calls[0].absSandboxDir, wantSandboxDir, "Incorrect sandbox dir")
			assert.Equal(t, cmdRunner.Calls[0].workDir, wantCheckoutDir, "Incorrect work dir")
			assert.DeepEqual(t, cmdRunner.Calls[0].cmd, tc.buildCmd, "Incorrect build command")
			assert.ElementsMatch(t,
				cmdRunner.Calls[0].env,
				[]string{
					"CI=true",
					fmt.Sprintf("HOME=/mockdir/%d", tc.buildID),
					"PATH=/usr/lib/go/bin:/usr/local/bin:/usr/bin",
					"ENV_VAR_A=env A",
					"ENV_VAR_B=env B",
					"BUILD_SECRET_A=build A",
					"BUILD_SECRET_B=build B",
				},
				"Incorrect build env",
			)

			if tc.shouldDeploy {
				// Check deploy cmd
				assert.Equal(t, cmdRunner.Calls[1].buildID, tc.buildID, "Incorrect build ID")
				assert.Equal(t, cmdRunner.Calls[1].absSandboxDir, wantSandboxDir, "Incorrect sandbox dir")
				assert.Equal(t, cmdRunner.Calls[1].workDir, wantCheckoutDir, "Incorrect work dir")
				assert.DeepEqual(t, cmdRunner.Calls[1].cmd, tc.deployCmd, "Incorrect deploy command")
				assert.ElementsMatch(t,
					cmdRunner.Calls[1].env,
					[]string{
						"CI=true",
						fmt.Sprintf("HOME=/mockdir/%d", tc.buildID),
						"PATH=/usr/lib/go/bin:/usr/local/bin:/usr/bin",
						"ENV_VAR_A=env A",
						"ENV_VAR_B=env B",
						"DEPLOY_SECRET_A=deploy A",
						"DEPLOY_SECRET_B=deploy B",
					},
					"Incorrect deploy env",
				)
			}
		})
	}
}
