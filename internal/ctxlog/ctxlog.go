package ctxlog

import (
	"context"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"
)

type loggerKey struct{}

func ContextWithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, logger)
}

func FromContext(ctx context.Context) *slog.Logger {
	logger := ctx.Value(loggerKey{})
	if logger == nil {
		return slog.Default()
	}

	return logger.(*slog.Logger)
}

func Middleware(next http.Handler, omitQueryPaths []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := FromContext(ctx)

		logFields := []any{
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("method", r.Method),
		}

		uriParts := strings.SplitN(r.RequestURI, "?", 2)
		logFields = append(logFields, slog.String("path", uriParts[0]))
		if !slices.Contains(omitQueryPaths, uriParts[0]) && len(uriParts) == 2 {
			logFields = append(logFields, slog.String("query", "?"+uriParts[1]))
		}

		log = log.With(logFields...)

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
