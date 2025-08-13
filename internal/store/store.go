package store

import "github.com/jackc/pgx/v5"

type PGStore struct {
	Repo  RepoStore
	Build BuildStore
	Log   LogStore
}

func NewPGStore(conn *pgx.Conn) PGStore {
	return PGStore{
		Repo:  pgRepoStore{conn},
		Build: pgBuildStore{conn},
		Log:   pgLogStore{conn},
	}
}
