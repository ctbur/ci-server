package store

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type LogEntry struct {
	ID        uint64
	BuildID   uint64
	Timestamp time.Time
	Text      string
}

type LogStore interface {
	Create(ctx context.Context, log LogEntry) error
}

type pgLogStore struct {
	conn *pgx.Conn
}

func (s pgLogStore) Create(ctx context.Context, log LogEntry) error {
	_, err := s.conn.Exec(
		ctx,
		`INSERT INTO logs (
			build_id,
			timestamp,
			text
		) VALUES (
			$1, $2, $3
		)`,
		log.BuildID,
		log.Timestamp,
		log.Text,
	)
	return err
}
