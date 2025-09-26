package build

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type Dispatcher struct {
	Builds   BuildRepository
	Logs     LogConsumer
	Cfg      *config.Config
	builders []Builder
}

const dispatchPollPeriod = 500 * time.Millisecond

func (d *Dispatcher) Run(log *slog.Logger, ctx context.Context) error {
	for {
		select {
		case <-time.After(dispatchPollPeriod):
			d.dispatch(log, ctx)

		case <-ctx.Done():
			return nil
		}
	}
}

type BuildRepository interface {
	GetPendingBuilds(ctx context.Context) ([]store.BuildWithRepoMeta, error)
	UpdateBuildState(ctx context.Context, buildID uint64, state store.BuildState) error
}

func (d *Dispatcher) dispatch(log *slog.Logger, ctx context.Context) {
	// Handle finished builds
	var newBuilders []Builder
	for _, builder := range d.builders {
		select {
		case res := <-builder.resChan:
			// Publish final build result
			d.Builds.UpdateBuildState(ctx, builder.buildID, res.buildState)

			if res.err != nil {
				log.ErrorContext(ctx, "internal error during build", slog.Any("error", res.err))
			}

		default:
			newBuilders = append(newBuilders, builder)
		}
	}
	d.builders = newBuilders

	// Handle pending builds
	pendingBuilds, err := d.Builds.GetPendingBuilds(ctx)
	if err != nil {
		log.ErrorContext(ctx, "failed to get pending builds", slog.Any("error", err))
		return
	}

	for i := range pendingBuilds {
		bld := &pendingBuilds[i]

		// Update start time for build
		now := time.Now()
		bld.Started = &now
		d.Builds.UpdateBuildState(ctx, bld.ID, bld.BuildState)

		// Start build routine
		// TODO: limit builds by number or resource usage
		builder, err := d.runBuilder(bld)
		if err != nil {
			log.ErrorContext(ctx, "failed to run builder", slog.Any("error", err))
			continue
		}
		d.builders = append(d.builders, *builder)
	}
}

type Builder struct {
	buildID uint64
	ctx     context.Context
	resChan chan BuildResult
}

type BuildResult struct {
	buildState store.BuildState
	err        error
}

func (d *Dispatcher) runBuilder(bld *store.BuildWithRepoMeta) (*Builder, error) {
	repoConfig := d.Cfg.GetRepoConfig(bld.Owner, bld.Name)
	if repoConfig == nil {
		return nil, fmt.Errorf("missing build config for %s/%s", bld.Owner, bld.Name)
	}

	resChan := make(chan BuildResult)

	go func() {
		exitCode, err := build(bld, d.Cfg.BuildDir, repoConfig.BuildCommand, d.Logs)

		buildState := bld.BuildState

		// Set finish time
		now := time.Now()
		buildState.Finished = &now

		// Determine store.BuildResult
		var result store.BuildResult
		if err != nil {
			result = store.BuildResultError
		} else if exitCode != 0 {
			result = store.BuildResultFailed
		} else {
			result = store.BuildResultSuccess
		}
		buildState.Result = &result

		resChan <- BuildResult{buildState, err}
	}()

	return &Builder{
		buildID: bld.ID,
		// TODO: pass context to build() to shut down or time out builds
		ctx:     context.TODO(),
		resChan: resChan,
	}, nil
}
