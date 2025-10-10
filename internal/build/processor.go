package build

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type Processor struct {
	Repos   config.RepoConfigs
	Builds  buildStore
	Builder builderController
	Dir     dataDir
}

func NewProcessor(repos config.RepoConfigs, dir DataDir, s store.PGStore) *Processor {
	return &Processor{
		Repos:   repos,
		Builds:  s,
		Dir:     &dir,
		Builder: &BuilderController{DataDir: dir.RootDir},
	}
}

const dispatchPollPeriod = 500 * time.Millisecond

func (p *Processor) Run(log *slog.Logger, ctx context.Context) error {
	for {
		select {
		case <-time.After(dispatchPollPeriod):
			p.process(log, ctx)

		case <-ctx.Done():
			return nil
		}
	}
}

type buildStore interface {
	GetPendingBuilds(ctx context.Context) ([]store.PendingBuild, error)
	StartBuild(ctx context.Context, buildID uint64, started time.Time, pid int, cacheID *uint64) error
	FinishBuild(ctx context.Context, buildID uint64, finished time.Time, result store.BuildResult, cacheBuildFiles bool) error
	ListBuilders(ctx context.Context) ([]store.Builder, error)
	ListBuildDirsInUse(ctx context.Context) ([]uint64, error)
}

type builderController interface {
	Start(repo config.RepoConfig, build store.PendingBuild, runDeploy bool) (int, error)
	IsRunning(pid int, buildID uint64) bool
}

type dataDir interface {
	ReadAndCleanExitCode(buildID uint64) (int, error)
	RetainBuildDirs(retainedIDs []uint64) ([]uint64, error)
}

func (p *Processor) process(log *slog.Logger, ctx context.Context) {
	// Handle finished builds
	runningBuilders, err := p.Builds.ListBuilders(ctx)
	if err != nil {
		log.ErrorContext(ctx, "failed to get running builds", slog.Any("error", err))
		return
	}

	for _, br := range runningBuilders {
		if p.Builder.IsRunning(br.PID, br.BuildID) {
			continue
		}

		// Update build result
		exitCode, err := p.Dir.ReadAndCleanExitCode(br.BuildID)
		var result store.BuildResult
		if err != nil {
			result = store.BuildResultError
			log.InfoContext(
				ctx, "Builder error",
				slog.Uint64("build_id", br.BuildID),
				slog.Any("error", err),
			)
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
			log.ErrorContext(
				ctx, "missing build config",
				slog.String("owner", br.Repo.Owner),
				slog.String("repo", br.Repo.Name),
			)
		}
		err = p.Builds.FinishBuild(ctx, br.BuildID, time.Now(), result, cacheBuildFiles)
		if err != nil {
			log.InfoContext(ctx, "failed to finish build", slog.Any("error", err))
			continue
		}

		log.InfoContext(
			ctx, "Finished build",
			slog.Uint64("build_id", br.BuildID),
			slog.Any("cache_id", br.CacheID),
			slog.Bool("made_cache", cacheBuildFiles),
			slog.Any("result", result),
		)
	}

	// Start new builds
	pendingBuilds, err := p.Builds.GetPendingBuilds(ctx)
	if err != nil {
		log.ErrorContext(ctx, "failed to get pending builds", slog.Any("error", err))
		return
	}

	for i := range pendingBuilds {
		b := pendingBuilds[i]

		repo := p.Repos.Get(b.Repo.Owner, b.Repo.Name)
		if repo == nil {
			log.ErrorContext(
				ctx, "missing build config",
				slog.String("owner", b.Repo.Owner),
				slog.String("repo", b.Repo.Name),
			)
			continue
		}

		// TODO: limit builds by number or resource usage

		// Don't run deploy if not on default branch
		runDeploy := b.Ref == fmt.Sprintf("refs/heads/%s", repo.DefaultBranch)
		pid, err := p.Builder.Start(*repo, b, runDeploy)
		if err != nil {
			log.ErrorContext(
				ctx,
				"failed to start builder",
				slog.Uint64("build_id", b.ID),
				slog.Any("error", err),
			)
			continue
		}

		// Update start time for build
		err = p.Builds.StartBuild(ctx, b.ID, time.Now(), pid, b.CacheID)
		if err != nil {
			log.ErrorContext(
				ctx,
				"failed to mark build as started",
				slog.Uint64("build_id", b.ID),
				slog.Any("error", err),
			)
		}
		log.InfoContext(
			ctx, "Started build",
			slog.Uint64("build_id", b.ID),
			slog.Any("cache_id", b.CacheID),
			slog.Int("pid", pid),
		)
	}

	// Clean up unused build dirs
	buildDirsInUse, err := p.Builds.ListBuildDirsInUse(ctx)
	if err != nil {
		log.ErrorContext(ctx, "Failed to list build dirs in use", slog.Any("error", err))
		return
	}

	deletedIDs, err := p.Dir.RetainBuildDirs(buildDirsInUse)
	if err != nil {
		log.ErrorContext(ctx, "Failed to delete unused build dirs", slog.Any("error", err))
	}
	if len(deletedIDs) > 0 {
		log.InfoContext(ctx, "Deleted unused build dirs", slog.Any("build_ids", deletedIDs))
	}
}
