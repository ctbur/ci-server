package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(conn *pgxpool.Pool) PGStore {
	return PGStore{conn}
}

type BuildWithRepoMeta struct {
	Build
	RepoMeta
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
