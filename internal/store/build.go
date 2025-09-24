package store

import (
	"context"
	"time"
)

type BuildResult string

const (
	// Build successful
	BuildResultSuccess BuildResult = "success"
	// Build itself failed
	BuildResultFailed BuildResult = "failure"
	// User canceled the build
	BuildResultCanceled BuildResult = "canceled"
	// Build timed out
	BuildResultTimeout BuildResult = "timeout"
	// CI encountered an error
	BuildResultError BuildResult = "error"
)

type BuildMeta struct {
	RepoID    uint64
	Number    uint64
	Link      string
	Ref       string
	CommitSHA string
	Message   string
	Author    string
}

type BuildState struct {
	Created  time.Time
	Started  *time.Time
	Finished *time.Time
	Result   *BuildResult
}

type Build struct {
	ID uint64
	BuildMeta
	BuildState
}

func (s PGStore) CreateBuild(ctx context.Context, build BuildMeta) (uint64, error) {
	var newID uint64

	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO builds (
			repo_id,
			number,
			link,
			ref,
			commit_sha,
			message,
			author,
			created
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8
		) RETURNING id`,
		build.RepoID,
		build.Number,
		build.Link,
		build.Ref,
		build.CommitSHA,
		build.Message,
		build.Author,
		time.Now(),
	).Scan(&newID)

	return newID, err
}

func (s PGStore) UpdateBuildState(ctx context.Context, buildID uint64, state BuildState) error {
	_, err := s.pool.Exec(
		ctx,
		`UPDATE builds
		SET created = $1, started = $2, finished = $3, result = $4
		WHERE id = $5`,
		state.Created,
		state.Started,
		state.Finished,
		state.Result,
		buildID,
	)
	return err
}
