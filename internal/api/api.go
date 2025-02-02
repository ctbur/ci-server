package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
)

type API struct {
}

func New() API {
	return API{}
}

func (a API) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(loggerMiddleware)

	r.Route("/webhook", func(r chi.Router) {
		r.Post("/", handleWebhook())
	})

	return r
}

type loggerKey struct{}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	logger := ctx.Value(loggerKey{})
	return logger.(*slog.Logger)
}

func loggerMiddleware(next http.Handler) http.Handler {
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

func handleWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := LoggerFromContext(r.Context())
		log.Info("HandleWebhook called")
		renderJSON(w, struct{}{}, 200)
	}
}

func renderJSON(w http.ResponseWriter, v interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}
