package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testUser = "ci-server"
const testPassword = "123456"
const dbName = "ci"

func initDatabase(dataDir string) error {
	pwFile, err := os.CreateTemp("", "pg_pw_")
	if err != nil {
		return fmt.Errorf("failed to create password file: %w", err)
	}
	_, err = pwFile.WriteString(testPassword)
	if err != nil {
		pwFile.Close()
		return fmt.Errorf("failed to write password file: %w", err)
	}
	_ = pwFile.Close()
	defer os.Remove(pwFile.Name())

	initCmd := exec.Command("initdb",
		"-D", dataDir,
		"--locale=C",
		"--encoding=UTF8",
		"--auth=scram-sha-256",
		"--username", testUser,
		"--pwfile", pwFile.Name())
	if out, err := initCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("initdb failed: %w\n%s", err, out)
	}
	return nil
}

func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func startDatabase(dataDir string, port int) error {
	// Check if server is already running
	statusCmd := exec.Command("pg_ctl", "status", "-D", dataDir)
	if err := statusCmd.Run(); err == nil {
		// Server is already running
		return nil
	}

	// -k "" disables Unix sockets, forcing TCP-only (avoids /run/postgresql permission issues)
	opts := fmt.Sprintf("-p %d -k \"\"", port)
	// Use -l to log to a file instead of stdout, preventing pipe hangs
	logFile := path.Join(dataDir, "postgresql.log")
	cmd := exec.Command("pg_ctl", "start", "-w", "-D", dataDir, "-o", opts, "-l", logFile)
	if err := cmd.Run(); err != nil {
		logContent, _ := os.ReadFile(logFile)
		return fmt.Errorf("pg_ctl start failed: %w\n%s", err, logContent)
	}
	return nil
}

func createDatabase(ctx context.Context, port int) (*pgxpool.Pool, error) {
	// Connect to 'postgres' database first to create our target database
	bootstrapURL := fmt.Sprintf(
		"postgresql://%s:%s@localhost:%d/postgres",
		testUser, testPassword, port,
	)

	var pool *pgxpool.Pool
	var err error
	for range 20 {
		pool, err = pgxpool.New(ctx, bootstrapURL)
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	_, err = pool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", dbName))
	if err != nil {
		var pgErr *pgconn.PgError
		// 42P04 = duplicate_database
		if !(errors.As(err, &pgErr) && pgErr.Code == "42P04") {
			pool.Close()
			return nil, fmt.Errorf("create database failed: %w", err)
		}
	}
	pool.Close()

	// Now connect to the actual target database
	dbURL := fmt.Sprintf(
		"postgresql://%s:%s@localhost:%d/%s",
		testUser, testPassword, port, dbName,
	)
	pool, err = pgxpool.New(ctx, dbURL)
	if err != nil {
		return nil, fmt.Errorf("connect to %s failed: %w", dbName, err)
	}

	return pool, nil
}

func stopDatabase(dataDir string) error {
	cmd := exec.Command("pg_ctl", "stop", "-D", dataDir, "-m", "fast")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pg_ctl stop failed: %w\n%s", err, out)
	}
	return nil
}

// Starts a persistent database
func StartDevDatabase(
	ctx context.Context, dataDir string, port int,
) (pool *pgxpool.Pool, cleanup func(), err error) {
	pgVersionPath := path.Join(dataDir, "PG_VERSION")
	if _, err := os.Stat(pgVersionPath); os.IsNotExist(err) {
		err = initDatabase(dataDir)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to init database: %w", err)
		}
	}

	err = startDatabase(dataDir, port)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start database: %w", err)
	}

	pool, err = createDatabase(ctx, port)
	if err != nil {
		_ = stopDatabase(dataDir)
		return nil, nil, fmt.Errorf("failed to create database: %w", err)
	}

	return pool, func() {
		pool.Close()
		err := stopDatabase(dataDir)
		if err != nil {
			log := ctxlog.FromContext(ctx)
			log.ErrorContext(ctx, "failed to stop database", slog.Any("error", err))
		}
	}, nil
}

// Starts a database with a temporary data dir and arbitrary port
func StartTestDatabase(
	ctx context.Context,
) (pool *pgxpool.Pool, cleanup func(), err error) {
	testDataDir, err := os.MkdirTemp("", "pgdata_test_")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp data dir: %w", err)
	}

	port, err := getFreePort()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get free port: %w", err)
	}

	pool, cleanup, err = StartDevDatabase(ctx, testDataDir, port)
	if err != nil {
		os.RemoveAll(testDataDir)
		return nil, nil, fmt.Errorf("failed to start test database: %w", err)
	}

	err = ApplyMigrations(ctx, pool)
	if err != nil {
		cleanup()
		os.RemoveAll(testDataDir)
		return nil, nil, fmt.Errorf("failed to apply migrations: %w", err)
	}

	return pool, func() {
		cleanup()
		os.RemoveAll(testDataDir)
	}, nil
}
