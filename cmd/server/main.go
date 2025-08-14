package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web"
	"github.com/jackc/pgx/v5"
)

func main() {
	conn, err := pgx.Connect(context.Background(), os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())
	pgStore := store.NewPGStore(conn)

	ctx := context.Background()

	_, err = conn.Exec(ctx, "DROP SCHEMA public CASCADE;")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to drop schema: %v\n", err)
		os.Exit(1)
	}

	_, err = conn.Exec(ctx, "CREATE SCHEMA public;")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create schema: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Schema 'public' recreated successfully.")

	migrationDir := "./migrations"

	files, err := os.ReadDir(migrationDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to read migrations directory: %v\n", err)
		os.Exit(1)
	}

	// Sort files to ensure they are applied in alphabetical order
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name() < files[j].Name()
	})

	for _, file := range files {
		path := filepath.Join(migrationDir, file.Name())

		sql, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to read migration file '%s': %v\n", path, err)
			os.Exit(1)
		}

		fmt.Printf("Running migration: %s\n", file.Name())

		_, err = conn.Exec(ctx, string(sql))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to run migration '%s': %v\n", file.Name(), err)
			os.Exit(1)
		}
	}

	var cfg config.Config
	if _, err := toml.DecodeFile("ci-config.toml", &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	bld := build.NewBuilder(cfg.BuildDir)

	for _, repoCfg := range cfg.Repos {
		repo, err := pgStore.Repo.Get(ctx, repoCfg.Owner, repoCfg.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to get repo %s/%s: %v\n", repoCfg.Owner, repoCfg.Name, err)
			os.Exit(1)
		}

		if repo != nil {
			continue
		}

		_, err = pgStore.Repo.Create(
			ctx,
			store.RepoMeta{
				Owner: repoCfg.Owner,
				Name:  repoCfg.Name,
			},
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create repo %s/%s: %v\n", repoCfg.Owner, repoCfg.Name, err)
			os.Exit(1)
		}
	}

	server := &http.Server{
		Addr:    ":8000",
		Handler: web.Handler(cfg, pgStore, bld),
	}

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server error", slog.Any("error", err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP shutdown error", slog.Any("error", err))
	}

	slog.Info("Shutdown complete.")
}
