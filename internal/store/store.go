package store

import (
	"context"

	"github.com/jackc/pgx/v5"
)

type PGStore struct {
	conn *pgx.Conn
}

func NewPGStore(conn *pgx.Conn) PGStore {
	return PGStore{conn}
}

type BuildWithRepoMeta struct {
	Build
	RepoMeta RepoMeta
}

func (s PGStore) ListBuildsWithRepo(ctx context.Context) ([]BuildWithRepoMeta, error) {
	rows, err := s.conn.Query(
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
			b.status,
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
			&b.Build.Status,
			&b.RepoMeta.Owner,
			&b.RepoMeta.Name,
		)
		return b, err
	})
}
