package store

import (
	"context"

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
}

var _ RepoStore = pgRepoStore{}

type pgRepoStore struct {
	conn *pgx.Conn
}

func (s pgRepoStore) Create(ctx context.Context, repo RepoMeta) (uint64, error) {
	var newID uint64

	err := s.conn.QueryRow(
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

func (s pgRepoStore) IncrementBuildCounter(ctx context.Context, repoID uint64) (uint64, error) {
	var newBuildCounter uint64

	err := s.conn.QueryRow(
		ctx,
		`UPDATE repos SET build_counter = build_counter + 1 RETURNING build_counter`,
	).Scan(&newBuildCounter)

	return newBuildCounter, err
}
