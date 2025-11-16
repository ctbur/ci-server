package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"syscall"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/github"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
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

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle SIGINT and SIGTERM.
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		sig := <-sigChan
		slog.Info("Received signal, shutting down...", slog.String("signal", sig.String()))
		cancel()
	}()

	// Load configuration
	configFile := path.Join(*configDir, "ci-config.toml")
	cfg, err := config.Load(os.Getenv("CI_SERVER_SECRET_KEY"), configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	cfg.DevMode = false
	if os.Getenv("CI_SERVER_DEV") == "1" {
		slog.Info("Starting in development mode based on CI_SERVER_DEV=1")
		slog.Warn("Development mode is not secure and should not be used in production!")
		cfg.DevMode = true
	}

	// Set global logger for context
	logLevel := slog.LevelInfo
	if cfg.DevMode {
		logLevel = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
	ctx = ctxlog.ContextWithLogger(ctx, log)

	// Load user authentication
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

	// Connect to database
	var postgresURL string
	if cfg.DevMode {
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

	pool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err)
	}
	defer pool.Close()

	err = store.ApplyMigrations(slog.Default(), ctx, pool)
	if err != nil {
		return err
	}
	slog.Info("Schema 'public' recreated successfully")

	db := store.NewPGStore(pool)
	err = store.InitRepositories(ctx, &db, cfg)
	if err != nil {
		return fmt.Errorf("failed to init repositories: %w", err)
	}

	fs := store.FSStore{
		RootDir: cfg.DataDir,
	}
	if err := fs.CreateRootDirs(); err != nil {
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

	processor := build.NewProcessor(cfg, &fs, &db, githubApp)
	go processor.Run(ctx)

	err = web.RunServer(ctx, 8000, cfg, userAuth, &db, &fs)
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
