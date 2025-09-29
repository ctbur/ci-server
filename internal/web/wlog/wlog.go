package wlog

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"
)

type loggerKey struct{}

func FromContext(ctx context.Context) *slog.Logger {
	logger := ctx.Value(loggerKey{})
	if logger == nil {
		return slog.Default()
	}

	return logger.(*slog.Logger)
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := slog.New(slog.NewTextHandler(os.Stdout, nil))
		log = log.With(
			slog.String("method", r.Method),
			slog.String("request", r.RequestURI),
		)

		ctx = context.WithValue(ctx, loggerKey{}, log)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(ctx))
		end := time.Now()

		duration := end.Sub(start).Milliseconds()

		log.Info(
			"request handled",
			slog.Int64("latency", duration),
			slog.String("time", end.Format(time.RFC3339)),
		)
	})
}
