package store

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path"
	"testing"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"
)

func StartTestDatabase(
	t *testing.T, ctx context.Context, repoDir string,
) (err error, pool *pgxpool.Pool, cleanup func()) {
	var tempDir string
	var postgres *embeddedpostgres.EmbeddedPostgres

	// Cleanup in case of error
	defer func() {
		if err != nil {
			if pool != nil {
				pool.Close()
			}
			if postgres != nil {
				_ = postgres.Stop()
			}
			if tempDir != "" {
				os.RemoveAll(tempDir)
			}
		}
	}()

	// Acquire temp dir
	tempDir, err = os.MkdirTemp(os.TempDir(), "ci-server-test")
	if err != nil {
		return fmt.Errorf("failed to create temp dir for embedded Postgres: %w", err), nil, nil
	}

	// Acquire free port
	port, err := getFreePort()

	postgres = embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Username("ci-server").
			Password("123456").
			Database("ci").
			BinariesPath(path.Join(repoDir, "./data/postgres/extracted")).
			// TODO: what would be a clean way to show the DB logs?
			Logger(io.Discard).
			RuntimePath(tempDir).
			Port(port),
	)

	err = postgres.Start()
	if err != nil {
		return fmt.Errorf("failed to start embedded Postgres: %v\n", err), nil, nil
	}

	databaseUrl := fmt.Sprintf(
		"postgresql://ci-server:123456@localhost:%d/ci",
		port,
	)

	pool, err = pgxpool.New(ctx, databaseUrl)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %v", err), nil, nil
	}

	log := loggerFromTesting(t)
	migrationsDir := path.Join(repoDir, "migrations")
	err = ApplyMigrations(log, ctx, pool, migrationsDir)
	if err != nil {
		return fmt.Errorf("failed to apply migrations: %v", err), nil, nil
	}

	return nil, pool, func() {
		pool.Close()
		_ = postgres.Stop()
		os.RemoveAll(tempDir)
	}
}

func getFreePort() (uint32, error) {
	// Listen on port 0 to get a random available port
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return uint32(l.Addr().(*net.TCPAddr).Port), nil
}

type TestLogWriter struct {
	t *testing.T
}

func (w *TestLogWriter) Write(p []byte) (n int, err error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

func loggerFromTesting(t *testing.T) *slog.Logger {
	handler := slog.NewTextHandler(&TestLogWriter{t}, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   false,
		ReplaceAttr: nil,
	})
	return slog.New(handler)
}
