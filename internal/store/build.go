package store

import (
	"context"
	"time"
)

type BuildStatus string

const (
	// Not yet run
	BuildStatusPending BuildStatus = "pending"
	// Currently being run
	BuildStatusRunning BuildStatus = "running"
	// Build successful
	BuildStatusSuccess BuildStatus = "success"
	// Build itself failed
	BuildStatusFailed BuildStatus = "failure"
	// User canceled the build
	BuildstatusCanceled BuildStatus = "canceled"
	// Build timed out
	BuildStatusTimeout BuildStatus = "timeout"
	// CI encountered an error
	BuildStatusError BuildStatus = "error"
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
	Started  time.Time
	Finished time.Time
	Status   BuildStatus
}

type Build struct {
	ID uint64
	BuildMeta
	BuildState
}

func (s PGStore) CreateBuild(ctx context.Context, build BuildMeta) (uint64, error) {
	var newID uint64

	err := s.conn.QueryRow(
		ctx,
		`INSERT INTO builds (
			repo_id,
			number,
			link,
			ref,
			commit_sha,
			message,
			author,
			created,
			status
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id`,
		build.RepoID,
		build.Number,
		build.Link,
		build.Ref,
		build.CommitSHA,
		build.Message,
		build.Author,
		time.Now(),
		BuildStatusPending,
	).Scan(&newID)

	return newID, err
}
