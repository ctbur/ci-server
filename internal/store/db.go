package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func DropAllData(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE;")
	if err != nil {
		return fmt.Errorf("failed to drop schema: %v\n", err)
	}

	_, err = pool.Exec(ctx, "CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("failed to create schema: %v\n", err)
	}

	return nil
}

func ApplyMigrations(log *slog.Logger, ctx context.Context, pool *pgxpool.Pool, migrationsDir string) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("failed to aqcuire connection to run migrations: %v", err)
	}
	defer conn.Release()

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %v\n", err)
	}

	// Sort files to ensure they are applied in alphabetical order
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS migrations (
			name VARCHAR(255) PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	for _, file := range files {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return fmt.Errorf("failed to begin transaction: %w", err)
		}
		defer tx.Rollback(ctx)

		var count int
		err = tx.QueryRow(ctx, "SELECT COUNT(*) FROM migrations WHERE name = $1", file.Name()).Scan(&count)
		if err != nil {
			return fmt.Errorf("failed to check migration status: %w", err)
		}

		if count > 0 {
			log.InfoContext(ctx, "Migration already applied", slog.String("file", file.Name()))
			continue
		}

		path := filepath.Join(migrationsDir, file.Name())

		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file '%s': %v\n", path, err)
		}

		log.InfoContext(ctx, "Running migration", slog.String("file", file.Name()))

		_, err = tx.Exec(ctx, string(sql))
		if err != nil {
			return fmt.Errorf("failed to run migration '%s': %v\n", file.Name(), err)
		}

		_, err = tx.Exec(ctx, "INSERT INTO migrations (name) VALUES ($1)", file.Name())
		if err != nil {
			return fmt.Errorf("failed to record migration '%s': %w", file.Name(), err)
		}

		// Commit the transaction to finalize all changes.
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("failed to commit transaction: %w", err)
		}
	}

	return nil
}

func InitDatabase(ctx context.Context, pgStore *PGStore, cfg *config.Config) error {
	for _, repoCfg := range cfg.Repos {
		err := pgStore.CreateRepoIfNotExists(
			ctx,
			Repo{
				Owner: repoCfg.Owner,
				Name:  repoCfg.Name,
			},
		)
		if err != nil {
			return fmt.Errorf("failed to create repo %s/%s: %v\n", repoCfg.Owner, repoCfg.Name, err)
		}
	}

	return nil
}
