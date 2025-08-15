package store

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/jackc/pgx/v5"
)

func DropAllData(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, "DROP SCHEMA public CASCADE;")
	if err != nil {
		return fmt.Errorf("failed to drop schema: %v\n", err)
	}

	_, err = conn.Exec(ctx, "CREATE SCHEMA public;")
	if err != nil {
		return fmt.Errorf("failed to create schema: %v\n", err)
	}

	return nil
}

func ApplyMigrations(log *slog.Logger, ctx context.Context, conn *pgx.Conn, migrationsDir string) error {

	files, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %v\n", err)
	}

	// Sort files to ensure they are applied in alphabetical order
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, file := range files {
		path := filepath.Join(migrationsDir, file.Name())

		sql, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file '%s': %v\n", path, err)
		}

		slog.Info("Running migration", slog.String("file", file.Name()))

		_, err = conn.Exec(ctx, string(sql))
		if err != nil {
			return fmt.Errorf("failed to run migration '%s': %v\n", file.Name(), err)
		}
	}

	return nil
}

func InitDatabase(ctx context.Context, pgStore *PGStore, cfg *config.Config) error {
	for _, repoCfg := range cfg.Repos {
		repo, err := pgStore.Repo.Get(ctx, repoCfg.Owner, repoCfg.Name)
		if err != nil {
			return fmt.Errorf("failed to get repo %s/%s: %v\n", repoCfg.Owner, repoCfg.Name, err)
		}

		if repo != nil {
			continue
		}

		_, err = pgStore.Repo.Create(
			ctx,
			RepoMeta{
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
