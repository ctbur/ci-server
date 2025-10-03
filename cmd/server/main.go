package main

import (
	"context"
	"fmt"
	"html/template"
	"log/slog"
	"os"
	"path"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
	"github.com/ctbur/ci-server/v2/internal/web/ui"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var err error
	if len(os.Args) > 2 && os.Args[1] == "builder" {
		err = build.RunBuilderFromEnv()
	} else {
		err = runServer()
	}

	if err != nil {
		slog.Error("Fatal error", slog.Any("error", err))
		os.Exit(1)
	}
	os.Exit(0)
}

func runServer() error {
	var postgresURL string
	if os.Getenv("CI_SERVER_DEV") == "1" {
		slog.Info("Starting in development mode")
		err, embeddedPostgresURL, cleanup := startDevDatabase()
		if err != nil {
			return err
		}
		postgresURL = embeddedPostgresURL
		defer cleanup()
		slog.Info("Embedded Postgres started")
	} else {
		postgresURL = os.Getenv("CI_SERVER_POSTGRES_URL")
		if postgresURL == "" {
			return fmt.Errorf("CI_SERVER_POSTGRES_URL not set")
		}
		slog.Info("Starting in production mode")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	// store.DropAllData(ctx, conn)
	err = store.ApplyMigrations(slog.Default(), ctx, pool, "./migrations")
	if err != nil {
		return err
	}
	slog.Info("Schema 'public' recreated successfully")

	cfg, err := config.Load(os.Getenv("CI_SERVER_SECRET_KEY"), "ci-config.toml")
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	htpasswd, err := os.ReadFile("users.htpasswd")
	if err != nil {
		return fmt.Errorf("failed to load users.htpasswd: %v", err)
	}

	userAuth, err := auth.FromHtpasswd(string(htpasswd))
	if err != nil {
		return fmt.Errorf("failed to decode users.htpasswd: %v", err)
	}

	tmpl, err := template.New("main").Funcs(ui.TemplateFuncMap).
		ParseGlob("ui/templates/*.tmpl")
	if err != nil {
		return fmt.Errorf("failed to load templates: %v", err)
	}

	pgStore := store.NewPGStore(pool)
	store.InitDatabase(ctx, &pgStore, cfg)

	processor := build.Processor{
		Builds: pgStore,
		Cfg:    cfg,
	}
	go processor.Run(slog.Default(), ctx)

	logStore := store.LogStore{
		LogDir: path.Join(cfg.DataDir, "logs"),
	}
	handler := web.Handler(cfg, userAuth, pgStore, logStore, tmpl, "ui/static/")
	web.RunServer(slog.Default(), handler, 8000)
	return nil
}

func startDevDatabase() (error, string, func()) {
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
	err := postgres.Start()
	if err != nil {
		return fmt.Errorf("failed to start embedded Postgres: %v\n", err), "", nil
	}
	return nil, "postgresql://ci-server:123456@localhost:5432/ci", func() {
		err := postgres.Stop()
		if err != nil {
			slog.Error("failed to stop embedded Postgres", slog.Any("error", err))
			return
		}
		slog.Info("embedded Postgres stopped")
	}
}
