package test

import (
	"log/slog"
	"testing"
)

type TestLogWriter struct {
	t *testing.T
}

func (w *TestLogWriter) Write(p []byte) (n int, err error) {
	w.t.Logf("%s", p)
	return len(p), nil
}

func Logger(t *testing.T) *slog.Logger {
	handler := slog.NewTextHandler(&TestLogWriter{t}, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		AddSource:   false,
		ReplaceAttr: nil,
	})
	return slog.New(handler)
}
