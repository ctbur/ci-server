package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(conn *pgxpool.Pool) DBStore {
	return DBStore{conn}
}

type Repo struct {
	Owner string
	Name  string
}

func (db DBStore) CreateRepoIfNotExists(ctx context.Context, repo Repo) error {
	_, err := db.pool.Exec(
		ctx,
		`INSERT INTO repos (owner, name)
		VALUES ($1, $2)
		ON CONFLICT (owner, name) DO NOTHING`,
		repo.Owner,
		repo.Name,
	)
	return err
}

func (db DBStore) CountRepos(ctx context.Context) (uint64, error) {
	var count uint64
	err := db.pool.QueryRow(ctx, `SELECT COUNT(*) FROM repos`).Scan(&count)
	return count, err
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
	Link      string
	Ref       string
	CommitSHA string
	Message   string
	Author    string
}

func (db DBStore) CreateBuild(
	ctx context.Context,
	repoOwner, repoName string,
	build BuildMeta,
	ts time.Time,
) (uint64, error) {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// TODO: implement serializable retry logic

	// Increment build counter
	var repoID uint64
	var buildNumber uint64

	err = tx.QueryRow(
		ctx,
		`UPDATE repos
		SET build_counter = build_counter + 1
		WHERE owner = $1 AND name = $2
		RETURNING id, build_counter`,
		repoOwner,
		repoName,
	).Scan(&repoID, &buildNumber)

	if err != nil {
		return 0, fmt.Errorf("failed to increment build counter: %w", err)
	}

	// Create build
	var newID uint64

	err = db.pool.QueryRow(
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
		repoID,
		buildNumber,
		build.Link,
		build.Ref,
		build.CommitSHA,
		build.Message,
		build.Author,
		ts,
	).Scan(&newID)

	if err != nil {
		return 0, fmt.Errorf("failed to create build: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return newID, err
}

func (db DBStore) StartBuild(
	ctx context.Context,
	buildID uint64,
	started time.Time,
	pid int,
	cacheID *uint64,
) error {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`UPDATE builds
		SET started = $1
		WHERE id = $2`,
		started,
		buildID,
	)
	if err != nil {
		return fmt.Errorf("failed to update build: %w", err)
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO builders (build_id, pid, cache_id)
		VALUES ($1, $2, $3)`,
		buildID, pid, cacheID,
	)
	if err != nil {
		return fmt.Errorf("failed to update builders: %w", err)
	}

	// TODO: return error if no rows were affected (build not found)
	return tx.Commit(ctx)
}

func (db DBStore) FinishBuild(
	ctx context.Context,
	buildID uint64,
	finished time.Time,
	result BuildResult,
	cacheBuildFiles bool,
) error {
	tx, err := db.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(
		ctx,
		`UPDATE builds
		SET finished = $1, result = $2
		WHERE id = $3`,
		finished,
		result,
		buildID,
	)
	if err != nil {
		return fmt.Errorf("failed to update build: %w", err)
	}

	if cacheBuildFiles {
		_, err = tx.Exec(
			ctx,
			`UPDATE repos
			SET cache_id = $1
			WHERE
				id = (
					SELECT repo_id
					FROM builds
					WHERE id = $1
				)
				AND
				(cache_id IS NULL OR cache_id < $1)`,
			buildID,
		)
		if err != nil {
			return fmt.Errorf("failed to update repo: %w", err)
		}
	}

	_, err = tx.Exec(
		ctx,
		`DELETE FROM builders WHERE build_id = $1`,
		buildID,
	)
	if err != nil {
		return fmt.Errorf("failed to update builders: %w", err)
	}

	// TODO: return error if no rows were affected (build not found)
	return tx.Commit(ctx)
}

type Build struct {
	ID       uint64
	RepoID   uint64
	Number   uint64
	Created  time.Time
	Started  *time.Time
	Finished *time.Time
	Result   *BuildResult
	Repo     Repo
	BuildMeta
}

var ErrNoBuild error = errors.New("build does not exist")

func (db DBStore) GetBuild(ctx context.Context, buildID uint64) (*Build, error) {
	var b Build

	err := db.pool.QueryRow(
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
		&b.ID,
		&b.RepoID,
		&b.Number,
		&b.Link,
		&b.Ref,
		&b.CommitSHA,
		&b.Message,
		&b.Author,
		&b.Created,
		&b.Started,
		&b.Finished,
		&b.Result,
		&b.Repo.Owner,
		&b.Repo.Name,
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoBuild
	}

	return &b, err
}

func (db DBStore) ListBuilds(
	ctx context.Context, page uint, pageSize uint,
) ([]Build, error) {
	rows, err := db.pool.Query(
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
		ORDER BY b.id DESC
		LIMIT $1
		OFFSET $2`,
		pageSize, page*pageSize,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Build, error) {
		var b Build
		err := rows.Scan(
			&b.ID,
			&b.RepoID,
			&b.Number,
			&b.Link,
			&b.Ref,
			&b.CommitSHA,
			&b.Message,
			&b.Author,
			&b.Created,
			&b.Started,
			&b.Finished,
			&b.Result,
			&b.Repo.Owner,
			&b.Repo.Name,
		)
		return b, err
	})
}

func (db DBStore) CountBuilds(ctx context.Context) (uint64, error) {
	var count uint64
	err := db.pool.QueryRow(ctx, `SELECT COUNT(*) FROM builds`).Scan(&count)
	return count, err
}

type PendingBuild struct {
	ID        uint64
	CacheID   *uint64
	Repo      Repo
	Ref       string
	CommitSHA string
}

func (db DBStore) GetPendingBuilds(ctx context.Context) ([]PendingBuild, error) {
	rows, err := db.pool.Query(
		ctx,
		`SELECT
			b.id,
			b.ref,
			b.commit_sha,
			r.owner,
			r.name,
			r.cache_id
		FROM builds AS b
		INNER JOIN repos AS r ON b.repo_id = r.id
		WHERE b.started IS NULL AND b.finished IS NULL AND b.result IS NULL
		ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(
		rows,
		func(row pgx.CollectableRow) (PendingBuild, error) {
			b := PendingBuild{}
			err := row.Scan(
				&b.ID,
				&b.Ref,
				&b.CommitSHA,
				&b.Repo.Owner,
				&b.Repo.Name,
				&b.CacheID,
			)
			return b, err
		})
}

type Builder struct {
	PID       int
	BuildID   uint64
	Repo      Repo
	CommitSHA string
	Ref       string
	CacheID   *uint64
}

func (db DBStore) ListBuilders(ctx context.Context) ([]Builder, error) {
	rows, err := db.pool.Query(
		ctx,
		`SELECT
			br.pid,
			b.id,
			r.owner,
			r.name,
			b.commit_sha,
			b.ref,
			br.cache_id
		FROM builders AS br
		INNER JOIN builds AS b ON br.build_id = b.id
		INNER JOIN repos AS r ON b.repo_id = r.id
		ORDER BY id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(
		rows,
		func(row pgx.CollectableRow) (Builder, error) {
			b := Builder{}
			err := row.Scan(
				&b.PID,
				&b.BuildID,
				&b.Repo.Owner,
				&b.Repo.Name,
				&b.CommitSHA,
				&b.Ref,
				&b.CacheID,
			)
			return b, err
		})
}

func (db DBStore) ListBuildDirsInUse(ctx context.Context) ([]uint64, error) {
	rows, err := db.pool.Query(
		ctx,
		// Keep build dirs, cache dirs in use, and repo cache dir
		`(SELECT build_id FROM builders)
		UNION
		(SELECT cache_id FROM builders WHERE cache_id IS NOT NULL)
		UNION
		(SELECT cache_id FROM repos WHERE cache_id IS NOT NULL)
		ORDER BY 1`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(
		rows,
		func(row pgx.CollectableRow) (uint64, error) {
			var id uint64
			err := row.Scan(&id)
			return id, err
		})
}
