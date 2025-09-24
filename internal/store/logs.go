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

func (s PGStore) CreateLog(ctx context.Context, log LogEntry) error {
	_, err := s.pool.Exec(
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

func (s PGStore) GetLogs(ctx context.Context, buildID uint64, fromLogID uint64) ([]LogEntry, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT id, timestamp, text FROM logs WHERE build_id = $1 AND id >= $2 ORDER BY id ASC`,
		buildID,
		fromLogID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (LogEntry, error) {
		log := LogEntry{BuildID: buildID}
		err := row.Scan(&log.ID, &log.Timestamp, &log.Text)
		return log, err
	})
}
