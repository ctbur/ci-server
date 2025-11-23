package build

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/github"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type mockBuildStore struct {
	pendingBuilds   []store.PendingBuild
	builders        []store.Builder
	buildDirsInUse  []uint64
	getPendingErr   error
	listBuildersErr error
	listDirsErr     error
	startBuildErr   error
	finishBuildErr  error
	startedBuilds   []startedBuild
	finishedBuilds  []finishedBuild
}

type startedBuild struct {
	BuildID uint64
	PID     int
	CacheID *uint64
}

type finishedBuild struct {
	BuildID         uint64
	Result          store.BuildResult
	CacheBuildFiles bool
}

func (m *mockBuildStore) GetPendingBuilds(
	ctx context.Context,
) ([]store.PendingBuild, error) {
	return m.pendingBuilds, m.getPendingErr
}

func (m *mockBuildStore) StartBuild(
	ctx context.Context, buildID uint64, started time.Time, pid int, cacheID *uint64,
) error {
	m.startedBuilds = append(m.startedBuilds, startedBuild{buildID, pid, cacheID})
	return m.startBuildErr
}

func (m *mockBuildStore) FinishBuild(
	ctx context.Context, buildID uint64, finished time.Time, result store.BuildResult, cacheBuildFiles bool,
) error {
	m.finishedBuilds = append(m.finishedBuilds, finishedBuild{buildID, result, cacheBuildFiles})
	return m.finishBuildErr
}

func (m *mockBuildStore) ListBuilders(ctx context.Context) ([]store.Builder, error) {
	return m.builders, m.listBuildersErr
}

func (m *mockBuildStore) ListBuildDirsInUse(ctx context.Context) ([]uint64, error) {
	return m.buildDirsInUse, m.listDirsErr
}

type mockBuilderController struct {
	runningPIDs map[int]uint64
	startPID    int
	startErr    error
	startCalls  []builderStart
}

type builderStart struct {
	Repo        config.RepoConfig
	AccessToken string
	Build       store.PendingBuild
	RunDeploy   bool
}

func (m *mockBuilderController) Start(
	repo config.RepoConfig, accessToken string, build store.PendingBuild, runDeploy bool,
) (int, error) {
	m.startCalls = append(m.startCalls, builderStart{repo, accessToken, build, runDeploy})
	return m.startPID, m.startErr
}

func (m *mockBuilderController) IsRunning(pid int, buildID uint64) bool {
	if m.runningPIDs == nil {
		return false
	}
	id, ok := m.runningPIDs[pid]
	return ok && id == buildID
}

type mockFSStore struct {
	exitCodes    map[uint64]int
	exitCodeErr  error
	retainedIDs  []uint64
	deletedIDs   []uint64
	retainErr    error
	retainCalled bool
}

func (m *mockFSStore) ReadAndCleanExitCode(buildID uint64) (int, error) {
	if m.exitCodeErr != nil {
		return 0, m.exitCodeErr
	}
	code, ok := m.exitCodes[buildID]
	if !ok {
		return 0, errors.New("exit code not found")
	}
	return code, nil
}

func (m *mockFSStore) RetainBuildDirs(retainedIDs []uint64) ([]uint64, error) {
	m.retainCalled = true
	m.retainedIDs = retainedIDs
	return m.deletedIDs, m.retainErr
}

type mockGitHubClient struct {
	token       string
	tokenErr    error
	statusCalls []commitStatus
	createErr   error
}

type commitStatus struct {
	Owner       string
	Repo        string
	SHA         string
	State       github.CommitState
	Description string
}

func (m *mockGitHubClient) GetInstallationToken(
	ctx context.Context, id github.InstallationID,
) (string, error) {
	return m.token, m.tokenErr
}

func (m *mockGitHubClient) CreateCommitStatus(
	ctx context.Context, id github.InstallationID, owner, repo, sha string,
	state github.CommitState, desc, url, ctxStr string,
) error {
	m.statusCalls = append(m.statusCalls, commitStatus{owner, repo, sha, state, desc})
	return m.createErr
}

type fixture struct {
	scheduler *Scheduler
	store     *mockBuildStore
	builder   *mockBuilderController
	fs        *mockFSStore
	github    *mockGitHubClient
}

func newFixture() *fixture {
	store := &mockBuildStore{}
	builder := &mockBuilderController{startPID: 12345}
	fs := &mockFSStore{exitCodes: make(map[uint64]int)}
	gh := &mockGitHubClient{token: "test-token"}

	return &fixture{
		scheduler: &Scheduler{
			HostURL: "https://ci.example.com",
			Repos:   config.RepoConfigs{{Owner: "testowner", Name: "testrepo", DefaultBranch: "main"}},
			Builds:  store,
			Builder: builder,
			FS:      fs,
			GitHub:  gh,
		},
		store:   store,
		builder: builder,
		fs:      fs,
		github:  gh,
	}
}

func (f *fixture) withFinishedBuild(buildID uint64, pid int, ref string, exitCode int) *fixture {
	f.store.builders = []store.Builder{{
		BuildID:   buildID,
		PID:       pid,
		Repo:      store.Repo{Owner: "testowner", Name: "testrepo"},
		Ref:       ref,
		CommitSHA: "abc123",
	}}
	f.builder.runningPIDs = nil // not running
	f.fs.exitCodes[buildID] = exitCode
	return f
}

func (f *fixture) withPendingBuild(buildID uint64, owner, name, ref string) *fixture {
	f.store.pendingBuilds = []store.PendingBuild{{
		ID:   buildID,
		Repo: store.Repo{Owner: owner, Name: name},
		Ref:  ref,
	}}
	return f
}

func TestSchedule_FinishBuilds_Results(t *testing.T) {
	tests := []struct {
		name       string
		exitCode   int
		exitErr    error
		wantResult store.BuildResult
		wantState  github.CommitState
	}{
		{
			name:       "success",
			exitCode:   0,
			wantResult: store.BuildResultSuccess,
			wantState:  github.CommitStateSuccess,
		},
		{
			name:       "failure",
			exitCode:   1,
			wantResult: store.BuildResultFailed,
			wantState:  github.CommitStateFailure,
		},
		{
			name:       "error reading exit code",
			exitErr:    errors.New("file not found"),
			wantResult: store.BuildResultError,
			wantState:  github.CommitStateError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			f.store.builders = []store.Builder{{
				BuildID:   1,
				PID:       100,
				Repo:      store.Repo{Owner: "testowner", Name: "testrepo"},
				Ref:       "refs/heads/main",
				CommitSHA: "abc123",
			}}
			f.builder.runningPIDs = nil
			if tt.exitErr != nil {
				f.fs.exitCodeErr = tt.exitErr
			} else {
				f.fs.exitCodes[1] = tt.exitCode
			}

			f.scheduler.schedule(context.Background())

			assert.Equal(t, len(f.store.finishedBuilds), 1, "incorrect number of finished builds").Fatal()
			assert.Equal(t, f.store.finishedBuilds[0].Result, tt.wantResult, "incorrect build result")
			assert.Equal(t, len(f.github.statusCalls), 1, "incorrect number of status updates").Fatal()
			assert.Equal(t, f.github.statusCalls[0].State, tt.wantState, "incorrect commit state")
		})
	}
}

func TestSchedule_FinishBuilds_Caching(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		wantCache bool
	}{
		{"default branch caches", "refs/heads/main", true},
		{"feature branch no cache", "refs/heads/feature", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture().withFinishedBuild(1, 100, tt.ref, 0)
			f.scheduler.schedule(context.Background())

			assert.Equal(t, len(f.store.finishedBuilds), 1, "incorrect number of finished builds").Fatal()
			assert.Equal(t, f.store.finishedBuilds[0].CacheBuildFiles, tt.wantCache, "incorrect cache decision")
		})
	}
}

func TestSchedule_StartBuilds_Deploy(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		wantDeploy bool
	}{
		{"default branch deploys", "refs/heads/main", true},
		{"feature branch no deploy", "refs/heads/feature", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture().withPendingBuild(10, "testowner", "testrepo", tt.ref)
			f.scheduler.schedule(context.Background())

			assert.Equal(t, len(f.builder.startCalls), 1, "incorrect number of started builds").Fatal()
			assert.Equal(t, f.builder.startCalls[0].RunDeploy, tt.wantDeploy, "incorrect deploy decision")
		})
	}
}

func TestSchedule_StillRunning_NotFinished(t *testing.T) {
	f := newFixture()
	f.store.builders = []store.Builder{{BuildID: 1, PID: 100}}
	f.builder.runningPIDs = map[int]uint64{100: 1}

	f.scheduler.schedule(context.Background())

	assert.Equal(t, len(f.store.finishedBuilds), 0, "should have no finished builds")
}

func TestSchedule_FullCycle_StartAndFinish(t *testing.T) {
	f := newFixture()
	ctx := context.Background()

	// First schedule: start a pending build
	f.store.pendingBuilds = []store.PendingBuild{{
		ID:             1,
		Repo:           store.Repo{Owner: "testowner", Name: "testrepo"},
		Ref:            "refs/heads/main",
		CommitSHA:      "abc123",
		InstallationID: 42,
	}}

	f.scheduler.schedule(ctx)

	assert.Equal(t, len(f.store.startedBuilds), 1, "incorrect number of started builds")
	assert.Equal(t, len(f.github.statusCalls), 1, "incorrect number of status calls").Fatal()
	assert.Equal(t, f.github.statusCalls[0].State, github.CommitStatePending, "incorrect commit state")

	// Second schedule: build finishes
	f.store.pendingBuilds = nil
	f.store.builders = []store.Builder{{
		BuildID:        1,
		PID:            f.builder.startPID,
		Repo:           store.Repo{Owner: "testowner", Name: "testrepo"},
		Ref:            "refs/heads/main",
		CommitSHA:      "abc123",
		InstallationID: 42,
	}}
	f.builder.runningPIDs = nil
	f.fs.exitCodes[1] = 0

	f.scheduler.schedule(ctx)

	assert.Equal(t, len(f.store.finishedBuilds), 1, "incorrect number of finished builds").Fatal()
	assert.Equal(t, f.store.finishedBuilds[0].Result, store.BuildResultSuccess, "incorrect build result")
	assert.Equal(t, len(f.github.statusCalls), 2, "incorrect number of status calls").Fatal()
	assert.Equal(t, f.github.statusCalls[1].State, github.CommitStateSuccess, "incorrect commit state")
}

func TestSchedule_StartBuilds_Errors(t *testing.T) {
	tests := []struct {
		name        string
		owner       string
		repo        string
		tokenErr    error
		builderErr  error
		wantStarted bool
	}{
		{
			name:        "success",
			owner:       "testowner",
			repo:        "testrepo",
			wantStarted: true,
		},
		{
			name:        "missing repo config",
			owner:       "unknown",
			repo:        "repo",
			wantStarted: false,
		},
		{
			name:        "token error",
			owner:       "testowner",
			repo:        "testrepo",
			tokenErr:    errors.New("auth failed"),
			wantStarted: false,
		},
		{
			name:        "builder error",
			owner:       "testowner",
			repo:        "testrepo",
			builderErr:  errors.New("spawn failed"),
			wantStarted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture().withPendingBuild(10, tt.owner, tt.repo, "refs/heads/main")
			f.github.tokenErr = tt.tokenErr
			f.builder.startErr = tt.builderErr

			f.scheduler.schedule(context.Background())

			if tt.wantStarted {
				assert.Equal(t, len(f.store.startedBuilds), 1, "incorrect number of started builds")
				assert.Equal(t, len(f.github.statusCalls), 1, "incorrect number of status updates").Fatal()
				assert.Equal(t, f.github.statusCalls[0].State, github.CommitStatePending, "incorrect commit state")
			} else {
				assert.Equal(t, len(f.store.startedBuilds), 0, "no builds should have started")
			}
		})
	}
}

func TestSchedule_Cleanup(t *testing.T) {
	tests := []struct {
		name      string
		dirsInUse []uint64
		deleted   []uint64
	}{
		{"retains in-use dirs", []uint64{1, 2, 3}, []uint64{4, 5}},
		{"no dirs in use", []uint64{}, []uint64{1, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture()
			f.store.buildDirsInUse = tt.dirsInUse
			f.fs.deletedIDs = tt.deleted

			f.scheduler.schedule(context.Background())

			assert.Equal(t, f.fs.retainCalled, true, "fs retain should have been called")
			assert.ElementsMatch(t, f.fs.retainedIDs, tt.dirsInUse, "incorrect retained dirs")
		})
	}
}

func TestRun_ContextCancellation(t *testing.T) {
	f := newFixture()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		f.scheduler.Run(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Error("Run did not exit on context cancellation")
	}
}
