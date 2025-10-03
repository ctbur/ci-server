package build

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path"
	"strconv"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type Processor struct {
	Builds BuildProcessingStore
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
	GetPendingBuilds(ctx context.Context) ([]store.BuildWithRepoMeta, error)
	MarkBuildStarted(ctx context.Context, buildID uint64, started time.Time) error
	MarkBuildFinished(ctx context.Context, buildID uint64, finished time.Time, result store.BuildResult) error
}

type Builder struct {
	PID       int     `json:"pid"`
	BuildID   uint64  `json:"build_id"`
	RepoOwner string  `json:"repo_owner"`
	RepoName  string  `json:"repo_name"`
	Ref       string  `json:"ref"`
	CacheID   *uint64 `json:"cache_id"`
}

func (p *Processor) process(log *slog.Logger, ctx context.Context) {
	// TODO: add more context to log statements

	// Handle finished builds
	var runningBuilders []Builder
	reposWithFinishedBuilds := make(map[string]struct{})

	previousRunningBuilds, err := readBuildIDs(p.Cfg.DataDir)
	if err != nil {
		log.ErrorContext(ctx, "failed to read IDs from build dir", slog.Any("error", err))
		return
	}

	for _, buildID := range previousRunningBuilds {
		builderFile := getBuilderFile(p.Cfg.DataDir, buildID)

		data, err := os.ReadFile(builderFile)
		if err != nil {
			log.ErrorContext(ctx, "failed to read builder file", slog.Any("error", err))
			continue
		}

		var builder Builder
		if err := json.Unmarshal(data, &builder); err != nil {
			log.ErrorContext(ctx, "failed to unmarshal builder file", slog.Any("error", err))
			continue
		}

		if isBuilderRunning(builder.PID, builder.BuildID) {
			runningBuilders = append(runningBuilders, builder)
			continue
		}

		// Update build state
		exitCode, err := readExitCodeFile(p.Cfg.DataDir, builder.BuildID)
		var result store.BuildResult
		if err != nil {
			result = store.BuildResultError
		} else if exitCode != 0 {
			result = store.BuildResultFailed
		} else {
			result = store.BuildResultSuccess
		}

		p.Builds.MarkBuildFinished(ctx, builder.BuildID, time.Now(), result)

		// If default branch, move files to cache, delete otherwise
		// TODO: add config entry for default branch
		if builder.Ref == "refs/heads/main" || builder.Ref == "refs/heads/master" {
			cacheRootDir := getCacheRootDir(p.Cfg.DataDir, builder.RepoOwner, builder.RepoName)
			err := os.MkdirAll(cacheRootDir, defaultPerms)
			if err != nil {
				log.ErrorContext(ctx, "failed to create cache dir for repo", slog.Any("error", err))
			}

			cacheDir := getCacheDir(p.Cfg.DataDir, builder.RepoOwner, builder.RepoName, buildID)
			buildFilesDir := getBuildFilesDir(p.Cfg.DataDir, buildID)
			err = os.Rename(buildFilesDir, cacheDir)
			if err != nil {
				log.ErrorContext(ctx, "failed to move build files to cache", slog.Any("error", err))
			}

			reposWithFinishedBuilds[fmt.Sprintf("%s/%s", builder.RepoOwner, builder.RepoName)] = struct{}{}
		}

		// Delete build dir
		buildDir := getBuildDir(p.Cfg.DataDir, builder.BuildID)
		if err := os.RemoveAll(buildDir); err != nil {
			log.ErrorContext(ctx, "failed to delete build dir", slog.Any("error", err))
		}
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
		repoConfig := p.Cfg.GetRepoConfig(bld.Owner, bld.Name)
		if repoConfig == nil {
			log.ErrorContext(ctx, "missing build config", slog.String("owner", bld.Owner), slog.String("repo", bld.Name))
			continue
		}

		// Update start time for build
		p.Builds.MarkBuildStarted(ctx, bld.ID, time.Now())

		// Determine build dirs and files
		buildDir := path.Join(p.Cfg.DataDir, "build", strconv.FormatUint(bld.ID, 10))

		if err := os.MkdirAll(buildDir, defaultPerms); err != nil {
			log.ErrorContext(ctx, "failed to create build dir", slog.Any("error", err))
			continue
		}
		cacheID, err := getLatestCache(p.Cfg.DataDir, bld.RepoMeta.Owner, bld.RepoMeta.Name)
		if err != nil {
			log.ErrorContext(ctx, "failed to get cache dir", slog.Any("error", err))
			continue
		}

		pid, err := startBuilder(
			BuilderParams{
				DataDir:   p.Cfg.DataDir,
				BuildID:   bld.ID,
				RepoOwner: bld.Owner,
				RepoName:  bld.Name,
				CommitSHA: bld.CommitSHA,
				Cmd:       repoConfig.BuildCommand,
				CacheID:   cacheID,
			},
		)
		if err != nil {
			log.ErrorContext(ctx, "failed to start builder", slog.Any("error", err))
			continue
		}

		// Write builder data to file
		builder := Builder{
			PID:       pid,
			BuildID:   bld.ID,
			RepoOwner: bld.RepoMeta.Owner,
			RepoName:  bld.RepoMeta.Name,
			Ref:       bld.Ref,
			CacheID:   cacheID,
		}

		builderFile := getBuilderFile(p.Cfg.DataDir, bld.ID)
		data, err := json.Marshal(&builder)
		if err != nil {
			log.ErrorContext(ctx, "failed to marshal builder", slog.Any("error", err))
		}

		if err := os.WriteFile(builderFile, data, 0o644); err != nil {
			log.ErrorContext(ctx, "failed to write builder file", slog.Any("error", err))
		}

		// Add to running builders
		runningBuilders = append(runningBuilders, builder)
	}

	// Clean up unused cache folders
	for repo := range reposWithFinishedBuilds {
		cacheRootDir := path.Join(p.Cfg.DataDir, repo, "cache")
		cacheDirs, err := os.ReadDir(cacheRootDir)
		if err != nil {
			log.ErrorContext(ctx, "failed to list repo cache folders", slog.Any("error", err))
			continue
		}

		// List all caches
		cacheIDs := make(map[uint64]struct{})
		var maxCacheID uint64
		for _, cacheDir := range cacheDirs {
			cacheID, err := strconv.ParseUint(cacheDir.Name(), 10, 64)
			if err != nil {
				continue
			}

			cacheIDs[cacheID] = struct{}{}
			maxCacheID = max(maxCacheID, cacheID)
		}

		if len(cacheIDs) == 0 {
			continue
		}

		// Remove caches to keep
		delete(cacheIDs, uint64(maxCacheID)) // Keep latest cache

		for _, builder := range runningBuilders {
			if builder.CacheID != nil {
				delete(cacheIDs, *builder.CacheID) // Keep caches in use
			}
		}

		// Delete remaining caches
		for cacheID := range cacheIDs {
			cacheDir := path.Join(cacheRootDir, strconv.FormatUint(cacheID, 10))
			err := os.RemoveAll(cacheDir)
			if err != nil {
				log.ErrorContext(ctx, "failed to delete cache dir", slog.Any("error", err))
			}
		}
	}
}

func readBuildIDs(dataDir string) ([]uint64, error) {
	return readDirIDs(getBuildRoot(dataDir))
}

func readDirIDs(dir string) ([]uint64, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}

	var ids []uint64
	for _, entry := range entries {
		id, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err != nil {
			continue
		}

		ids = append(ids, id)
	}

	return ids, nil
}

func readExitCodeFile(dataDir string, buildID uint64) (int, error) {
	data, err := os.ReadFile(getExitCodeFile(dataDir, buildID))
	if err != nil {
		return 0, err
	}

	exitCode, err := strconv.ParseUint(string(data), 10, 64)
	if err != nil {
		return 0, err
	}

	return int(exitCode), nil
}

const defaultPerms = 0700

func getLatestCache(dataDir string, repoOwner, repoName string) (*uint64, error) {
	// List all directories in cacheRootDir and find the one with the highest build ID
	entries, err := os.ReadDir(getCacheRootDir(dataDir, repoOwner, repoName))
	if os.IsNotExist(err) {
		// No cache dir exists yet
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read cache root dir: %w", err)
	}

	var latestBuildID *uint64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		buildID, err := strconv.ParseUint(entry.Name(), 10, 64)
		if err == nil {
			if latestBuildID == nil {
				latestBuildID = &buildID
			} else if buildID > *latestBuildID {
				latestBuildID = &buildID
			}
		}
	}

	return latestBuildID, nil
}
