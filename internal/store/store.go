package store

import "github.com/jackc/pgx/v5"

type PGStore struct {
	conn *pgx.Conn
}

func NewPGStore(conn *pgx.Conn) PGStore {
	return PGStore{conn}
}
