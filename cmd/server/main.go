package main

import (
	"context"
	"flag"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"path"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/github"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
	"github.com/ctbur/ci-server/v2/internal/web/ui"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	var err error
	if len(os.Args) >= 2 && os.Args[1] == "builder" {
		err = build.RunBuilder()
	} else {
		err = runServer()
	}

	if err != nil {
		slog.Error("Fatal error", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("Exited successfully")
	os.Exit(0)
}

func runServer() error {
	configDir := flag.String("config", ".", "Path to the directory containing ci-config.toml and users.htpasswd.")
	libDir := flag.String("lib", ".", "Path to the directory containing the SQL migrations and ui directories.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	flag.Parse()

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

	// store.DropAllData(ctx, pool)
	migrationsDir := path.Join(*libDir, "migrations")
	err = store.ApplyMigrations(slog.Default(), ctx, pool, migrationsDir)
	if err != nil {
		return err
	}
	slog.Info("Schema 'public' recreated successfully")

	configFile := path.Join(*configDir, "ci-config.toml")
	cfg, err := config.Load(os.Getenv("CI_SERVER_SECRET_KEY"), configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	htpasswdFile := path.Join(*configDir, "users.htpasswd")
	// sec: Path is from a trusted user
	htpasswd, err := os.ReadFile(htpasswdFile) // #nosec G304
	if err != nil {
		return fmt.Errorf("failed to load users.htpasswd: %v", err)
	}

	userAuth, err := auth.FromHtpasswd(string(htpasswd))
	if err != nil {
		return fmt.Errorf("failed to decode users.htpasswd: %v", err)
	}

	tmpl, err := template.New("main").Funcs(ui.TemplateFuncMap).
		ParseGlob(path.Join(*libDir, "ui/templates/*.tmpl"))
	if err != nil {
		return fmt.Errorf("failed to load templates: %v", err)
	}

	pgStore := store.NewPGStore(pool)
	err = store.InitRepositories(ctx, &pgStore, cfg)
	if err != nil {
		return fmt.Errorf("failed to init repositories: %w", err)
	}

	dataDir := build.DataDir{
		RootDir: cfg.DataDir,
	}
	if err := dataDir.CreateRootDirs(); err != nil {
		return fmt.Errorf("failed to create dirs under %s: %w", cfg.DataDir, err)
	}

	var githubApp *github.GitHubApp
	if cfg.GitHub != nil {
		privateKeyFile, err := os.Open(cfg.GitHub.PrivateKeyPath)
		if err != nil {
			return fmt.Errorf("failed to open GitHub app private key file: %w", err)
		}
		defer privateKeyFile.Close()

		ghAppPrivateKey, err := config.LoadRSAPrivateKey(privateKeyFile)
		if err != nil {
			return fmt.Errorf("failed to read GitHub app private key: %w", err)
		}
		githubApp = github.NewGitHubApp(
			&http.Client{},
			cfg.GitHub.AppID,
			cfg.GitHub.InstallationID,
			ghAppPrivateKey,
		)
	}

	processor := build.NewProcessor(cfg, &dataDir, pgStore, githubApp)
	go processor.Run(slog.Default(), ctx)

	logStore := store.LogStore{
		LogDir: path.Join(cfg.DataDir, "build-logs"),
	}
	staticFileDir := path.Join(*libDir, "ui/static/")
	handler := web.Handler(cfg, userAuth, pgStore, logStore, tmpl, staticFileDir)
	err = web.RunServer(slog.Default(), handler, 8000)
	if err != nil {
		return fmt.Errorf("error during web server execution: %w", err)
	}

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
