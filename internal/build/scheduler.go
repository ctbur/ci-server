package build

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/github"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type Scheduler struct {
	HostURL string
	Repos   config.RepoConfigs
	Builds  buildStore
	Builder builderController
	FS      schedulerFSStore
	GitHub  githubClient
}

type buildStore interface {
	GetPendingBuilds(ctx context.Context) ([]store.PendingBuild, error)
	StartBuild(ctx context.Context, buildID uint64, started time.Time, pid int, cacheID *uint64) error
	FinishBuild(ctx context.Context, buildID uint64, finished time.Time, result store.BuildResult, cacheBuildFiles bool) error
	ListBuilders(ctx context.Context) ([]store.Builder, error)
	ListBuildDirsInUse(ctx context.Context) ([]uint64, error)
}

type builderController interface {
	Start(repo config.RepoConfig, accessToken string, build store.PendingBuild, runDeploy bool) (int, error)
	IsRunning(pid int, buildID uint64) bool
}

type schedulerFSStore interface {
	ReadAndCleanExitCode(buildID uint64) (int, error)
	RetainBuildDirs(retainedIDs []uint64) ([]uint64, error)
}

type githubClient interface {
	GetInstallationToken(
		ctx context.Context,
		installationID github.InstallationID,
	) (string, error)
	CreateCommitStatus(
		ctx context.Context,
		installationID github.InstallationID,
		owner, repo, sha string,
		state github.CommitState,
		description string,
		targetURL string,
		contextStr string,
	) error
}

func NewScheduler(
	cfg *config.Config, fs *store.FSStore, db *store.DBStore, gh *github.GitHubApp,
) *Scheduler {
	return &Scheduler{
		HostURL: cfg.HostURL,
		Repos:   cfg.Repos,
		Builds:  db,
		FS:      fs,
		Builder: &BuilderController{FS: fs},
		GitHub:  gh,
	}
}

const dispatchPollPeriod = 500 * time.Millisecond

func (p *Scheduler) Run(ctx context.Context) {
	for {
		select {
		case <-time.After(dispatchPollPeriod):
			p.schedule(ctx)

		case <-ctx.Done():
			return
		}
	}
}

func (p *Scheduler) schedule(ctx context.Context) {
	log := ctxlog.FromContext(ctx)
	log = log.With(slog.String("component", "build_scheduler"))

	// Handle finished builds
	runningBuilders, err := p.Builds.ListBuilders(ctx)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get running builds", slog.Any("error", err))
		return
	}

	for _, br := range runningBuilders {
		if p.Builder.IsRunning(br.PID, br.BuildID) {
			continue
		}

		brLog := log.With(
			slog.String("owner", br.Repo.Owner),
			slog.String("repo", br.Repo.Name),
			slog.Uint64("build_id", br.BuildID),
		)
		p.finishBuild(brLog, ctx, br)
	}

	// Start new builds
	pendingBuilds, err := p.Builds.GetPendingBuilds(ctx)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get pending builds", slog.Any("error", err))
		return
	}

	for _, b := range pendingBuilds {
		// TODO: limit builds by number or resource usage

		bLog := log.With(
			slog.String("owner", b.Repo.Owner),
			slog.String("repo", b.Repo.Name),
			slog.Uint64("build_id", b.ID),
		)
		p.startBuild(bLog, ctx, b)

	}

	// Clean up unused build dirs
	buildDirsInUse, err := p.Builds.ListBuildDirsInUse(ctx)
	if err != nil {
		log.ErrorContext(ctx, "Failed to list build dirs in use", slog.Any("error", err))
		return
	}

	deletedIDs, err := p.FS.RetainBuildDirs(buildDirsInUse)
	if err != nil {
		log.ErrorContext(ctx, "Failed to delete unused build dirs", slog.Any("error", err))
	}
	if len(deletedIDs) > 0 {
		log.InfoContext(ctx, "Deleted unused build dirs", slog.Any("build_ids", deletedIDs))
	}
}

func (p *Scheduler) finishBuild(
	log *slog.Logger, ctx context.Context, br store.Builder,
) {
	// Update build result
	exitCode, err := p.FS.ReadAndCleanExitCode(br.BuildID)
	var result store.BuildResult
	if err != nil {
		result = store.BuildResultError
		log.ErrorContext(ctx, "Builder failed", slog.Any("error", err))
	} else if exitCode != 0 {
		result = store.BuildResultFailed
	} else {
		result = store.BuildResultSuccess
	}

	repo := p.Repos.Get(br.Repo.Owner, br.Repo.Name)
	cacheBuildFiles := false
	if repo != nil {
		// If default branch, move files to cache, delete otherwise
		cacheBuildFiles = br.Ref == fmt.Sprintf("refs/heads/%s", repo.DefaultBranch)
	} else {
		log.ErrorContext(ctx, "Missing repo config for finished build")
	}

	err = p.Builds.FinishBuild(ctx, br.BuildID, time.Now(), result, cacheBuildFiles)
	if err != nil {
		log.ErrorContext(ctx, "Failed to mark build as finished", slog.Any("error", err))
	}

	commitState := github.CommitStateError
	switch result {
	case store.BuildResultSuccess:
		commitState = github.CommitStateSuccess
	case store.BuildResultFailed, store.BuildResultCanceled, store.BuildResultTimeout:
		commitState = github.CommitStateFailure
	}
	err = p.GitHub.CreateCommitStatus(
		ctx,
		br.InstallationID,
		br.Repo.Owner,
		br.Repo.Name,
		br.CommitSHA,
		commitState,
		"Build finished",
		fmt.Sprintf("%s/builds/%d", p.HostURL, br.BuildID),
		"CI",
	)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create finished commit status", slog.Any("error", err))
	}

	log.InfoContext(
		ctx, "Finished build",
		slogCacheID(br.CacheID),
		slog.Bool("made_cache", cacheBuildFiles),
		slog.Any("result", result),
	)
}

func (p *Scheduler) startBuild(
	log *slog.Logger, ctx context.Context, b store.PendingBuild,
) {
	repo := p.Repos.Get(b.Repo.Owner, b.Repo.Name)
	if repo == nil {
		log.ErrorContext(ctx, "Missing repo config")
		return
	}

	repoAccessToken, err := p.GitHub.GetInstallationToken(ctx, b.InstallationID)
	if err != nil {
		log.ErrorContext(ctx, "Failed to get installation token", slog.Any("error", err))
	}

	// Don't run deploy if not on default branch
	runDeploy := b.Ref == fmt.Sprintf("refs/heads/%s", repo.DefaultBranch)
	pid, err := p.Builder.Start(*repo, repoAccessToken, b, runDeploy)
	if err != nil {
		log.ErrorContext(ctx, "Failed to start builder", slog.Any("error", err))
		return
	}

	err = p.Builds.StartBuild(ctx, b.ID, time.Now(), pid, b.CacheID)
	if err != nil {
		log.ErrorContext(ctx, "Failed to mark build as started", slog.Any("error", err))
	}

	err = p.GitHub.CreateCommitStatus(
		ctx,
		b.InstallationID,
		b.Repo.Owner,
		b.Repo.Name,
		b.CommitSHA,
		github.CommitStatePending,
		"Build started",
		fmt.Sprintf("%s/builds/%d", p.HostURL, b.ID),
		"CI",
	)
	if err != nil {
		log.ErrorContext(
			ctx,
			"Failed to create pending commit status",
			slog.Any("error", err),
		)
	}

	log.InfoContext(
		ctx, "Started build",
		slogCacheID(b.CacheID),
		slog.Int("pid", pid),
	)
}

func slogCacheID(cacheID *uint64) slog.Attr {
	if cacheID == nil {
		return slog.Any("cache_id", nil)
	}
	return slog.Uint64("cache_id", *cacheID)
}
