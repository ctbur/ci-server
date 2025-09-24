package store

import (
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(conn *pgxpool.Pool) PGStore {
	return PGStore{conn}
}
