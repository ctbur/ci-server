package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Fatal error", slog.Any("error", err))
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	postgres := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Username("ci-server").
			Password("123456").
			Database("ci").
			CachePath("./data/postgres/").
			RuntimePath("./data/postgres/extracted").
			// Configures data to be persistent because DataPath is outside RuntimePath
			DataPath("./data/postgres/data").
			BinariesPath("./data/postgres/extracted"),
	)
	databaseUrl := "postgresql://ci-server:123456@localhost:5432/ci"
	err := postgres.Start()
	if err != nil {
		return fmt.Errorf("failed to start embedded Postgres: %v\n", err)
	}
	defer func() {
		err := postgres.Stop()
		if err != nil {
			slog.Error("failed to stop embedded Postgres", slog.Any("error", err))
		}
	}()

	ctx := context.Background()
	conn, err := pgx.Connect(ctx, databaseUrl)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v\n", err)
	}
	defer conn.Close(ctx)

	err = store.ApplyMigrations(slog.Default(), ctx, conn, "./migrations")
	if err != nil {
		return err
	}
	slog.Info("Schema 'public' recreated successfully")

	var cfg config.Config
	if _, err := toml.DecodeFile("ci-config.toml", &cfg); err != nil {
		return fmt.Errorf("failed to load config: %v\n", err)
	}

	pgStore := store.NewPGStore(conn)
	store.InitDatabase(ctx, &pgStore, &cfg)

	bld := build.NewBuilder(cfg.BuildDir)

	handler := web.Handler(cfg, pgStore, bld)
	web.RunServer(slog.Default(), handler, 8000)
	return nil
}
