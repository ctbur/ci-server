package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type RepoMeta struct {
	Owner string
	Name  string
}

type RepoState struct {
	BuildCounter uint64
}

type Repo struct {
	ID uint64
	RepoMeta
	RepoState
}

type RepoStore interface {
	Create(ctx context.Context, repo RepoMeta) (uint64, error)
	IncrementBuildCounter(ctx context.Context, repoID uint64) (uint64, error)
	Get(ctx context.Context, owner, name string) (*Repo, error)
}

func (s PGStore) CreateRepo(ctx context.Context, repo RepoMeta) (uint64, error) {
	var newID uint64

	err := s.pool.QueryRow(
		ctx,
		`INSERT INTO repos (
			owner,
			name
		) VALUES (
			$1, $2
		) RETURNING id`,
		repo.Owner,
		repo.Name,
	).Scan(&newID)

	return newID, err
}

func (s PGStore) GetRepo(ctx context.Context, owner, name string) (*Repo, error) {
	var repo Repo

	err := s.pool.QueryRow(
		ctx,
		`SELECT id, owner, name, build_counter FROM repos
		WHERE owner = $1 AND name = $2`,
		owner,
		name,
	).Scan(
		&repo.ID,
		&repo.Owner,
		&repo.Name,
		&repo.BuildCounter,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &repo, err
}

func (s PGStore) IncrementBuildCounter(ctx context.Context, repoID uint64) (uint64, error) {
	var newBuildCounter uint64

	err := s.pool.QueryRow(
		ctx,
		`UPDATE repos SET build_counter = build_counter + 1 RETURNING build_counter`,
	).Scan(&newBuildCounter)

	return newBuildCounter, err
}

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

type BuildWithRepoMeta struct {
	Build
	RepoMeta
}

func (s PGStore) GetBuild(ctx context.Context, buildID uint64) (*BuildWithRepoMeta, error) {
	var b BuildWithRepoMeta

	err := s.pool.QueryRow(
		ctx,
		`SELECT
			b.id,
			b.repo_id,
			b.number,
			b.link,
			b.ref,
			b.commit_sha,
			b.message,
			b.author,
			b.created,
			b.started,
			b.finished,
			b.result,
			r.owner,
			r.name
		FROM builds AS b
		INNER JOIN repos AS r ON b.repo_id = r.id
		WHERE b.id = $1`,
		buildID,
	).Scan(
		&b.Build.ID,
		&b.Build.RepoID,
		&b.Build.Number,
		&b.Build.Link,
		&b.Build.Ref,
		&b.Build.CommitSHA,
		&b.Build.Message,
		&b.Build.Author,
		&b.Build.Created,
		&b.Build.Started,
		&b.Build.Finished,
		&b.Build.Result,
		&b.RepoMeta.Owner,
		&b.RepoMeta.Name,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return &b, err
}

func (s PGStore) ListBuilds(ctx context.Context) ([]BuildWithRepoMeta, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT
			b.id,
			b.repo_id,
			b.number,
			b.link,
			b.ref,
			b.commit_sha,
			b.message,
			b.author,
			b.created,
			b.started,
			b.finished,
			b.result,
			r.owner,
			r.name
		FROM builds AS b
		INNER JOIN repos AS r ON b.repo_id = r.id
		ORDER BY b.created DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (BuildWithRepoMeta, error) {
		var b BuildWithRepoMeta
		err := rows.Scan(
			&b.Build.ID,
			&b.Build.RepoID,
			&b.Build.Number,
			&b.Build.Link,
			&b.Build.Ref,
			&b.Build.CommitSHA,
			&b.Build.Message,
			&b.Build.Author,
			&b.Build.Created,
			&b.Build.Started,
			&b.Build.Finished,
			&b.Build.Result,
			&b.RepoMeta.Owner,
			&b.RepoMeta.Name,
		)
		return b, err
	})
}

func (s PGStore) GetPendingBuilds(
	ctx context.Context,
) ([]BuildWithRepoMeta, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT
			b.id,
			b.repo_id,
			b.number,
			b.link,
			b.ref,
			b.commit_sha,
			b.message,
			b.author,
			b.created,
			r.owner,
			r.name
		FROM builds AS b
		INNER JOIN repos AS r ON b.repo_id = r.id
		WHERE b.started IS NULL AND b.finished IS NULL AND b.result IS NULL
		ORDER BY id ASC
		LIMIT 10`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(
		rows,
		func(row pgx.CollectableRow) (BuildWithRepoMeta, error) {
			b := BuildWithRepoMeta{}
			err := row.Scan(
				&b.Build.ID,
				&b.Build.RepoID,
				&b.Build.Number,
				&b.Build.Link,
				&b.Build.Ref,
				&b.Build.CommitSHA,
				&b.Build.Message,
				&b.Build.Author,
				&b.Build.Created,
				&b.RepoMeta.Owner,
				&b.RepoMeta.Name,
			)
			return b, err
		})
}
