package build

import (
	"context"
	"log/slog"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type Processor struct {
	Builds BuildProcessingStore
	Dir    DataDir
	Cfg    *config.Config
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

type BuildProcessingStore interface {
	GetPendingBuilds(ctx context.Context) ([]store.PendingBuild, error)
	MarkBuildStarted(ctx context.Context, buildID uint64, started time.Time, pid int, cacheID *uint64) error
	MarkBuildFinished(ctx context.Context, buildID uint64, finished time.Time, result store.BuildResult, cacheBuildFiles bool) error
	ListBuilders(ctx context.Context) ([]store.Builder, error)
	ListBuildDirsInUse(ctx context.Context) ([]uint64, error)
}

type BuilderStore interface {
	ListBuildIDs() ([]uint64, error)
}

func (p *Processor) process(log *slog.Logger, ctx context.Context) {
	// TODO: ensure all errors are handled

	// Handle finished builds
	runningBuilds, err := p.Builds.ListBuilders(ctx)
	if err != nil {
		log.ErrorContext(ctx, "failed to get running builds", slog.Any("error", err))
		return
	}

	for _, builder := range runningBuilds {
		if isBuilderRunning(builder.PID, builder.BuildID) {
			continue
		}

		// Update build state
		exitCode, err := p.Dir.ReadAndCleanExitCode(builder.BuildID)
		var result store.BuildResult
		if err != nil {
			result = store.BuildResultError
			log.InfoContext(
				ctx, "Builder error",
				slog.Uint64("build_id", builder.BuildID),
				slog.Any("error", err),
			)
		} else if exitCode != 0 {
			result = store.BuildResultFailed
		} else {
			result = store.BuildResultSuccess
		}

		// If default branch, move files to cache, delete otherwise
		// TODO: add config entry for default branch
		cacheBuildFiles := builder.Ref == "refs/heads/main" || builder.Ref == "refs/heads/master"
		err = p.Builds.MarkBuildFinished(ctx, builder.BuildID, time.Now(), result, cacheBuildFiles)
		if err != nil {
			log.InfoContext(ctx, "failed to finish build", slog.Any("error", err))
			continue
		}

		log.InfoContext(
			ctx, "Finished build",
			slog.Uint64("build_id", builder.BuildID),
			slog.Any("cache_id", builder.CacheID),
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
		bld := &pendingBuilds[i]

		// TODO: limit builds by number or resource usage
		repoConfig := p.Cfg.GetRepoConfig(bld.Repo.Owner, bld.Repo.Name)
		if repoConfig == nil {
			log.ErrorContext(
				ctx, "missing build config",
				slog.String("owner", bld.Repo.Owner),
				slog.String("repo", bld.Repo.Name),
			)
			continue
		}

		pid, err := startBuilder(
			BuilderParams{
				DataDir:   p.Cfg.DataDir,
				BuildID:   bld.ID,
				CacheID:   bld.CacheID,
				RepoOwner: bld.Repo.Owner,
				RepoName:  bld.Repo.Name,
				CommitSHA: bld.CommitSHA,
				Cmd:       repoConfig.BuildCommand,
				Secrets:   repoConfig.BuildSecrets,
			},
		)
		if err != nil {
			log.ErrorContext(
				ctx,
				"failed to start builder",
				slog.Uint64("build_id", bld.ID),
				slog.Any("error", err),
			)
			continue
		}

		// Update start time for build
		p.Builds.MarkBuildStarted(ctx, bld.ID, time.Now(), pid, bld.CacheID)

		log.InfoContext(
			ctx, "Started build",
			slog.Uint64("build_id", bld.ID),
			slog.Any("cache_id", bld.CacheID),
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
