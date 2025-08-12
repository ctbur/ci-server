package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/go-chi/chi/v5"
)

type API struct {
}

func New() API {
	return API{}
}

func (a API) Handler(cfg config.Config, bld build.Builder) http.Handler {
	r := chi.NewRouter()
	r.Use(loggerMiddleware)

	r.Route("/webhook", func(r chi.Router) {
		r.Post("/", handleWebhook(cfg, bld))
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

func handleWebhook(cfg config.Config, bld build.Builder) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := LoggerFromContext(r.Context())
		log.Info("HandleWebhook called")

		commitSHA := "bbf29a537f3d2875bba4304b1543d4bf0278b6d9"
		result, err := bld.Build("godotengine", "godot", "master", commitSHA, []string{"scons", "platform=linux"})
		if result != nil {
			log.Info("Build completed", slog.Bool("success", result.Success), slog.Duration("duration", result.Duration))
		} else {
			log.Error("Build failed: %w", result)
		}

		if err != nil {
			renderError(w, err, 500)
		} else {
			renderStruct(w, struct{}{}, 200)
		}
	}
}

func renderStruct(w http.ResponseWriter, v any, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

func renderError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)

	errStr := fmt.Sprintf("%v", err)
	enc.Encode(struct {
		Error string `json:"error"`
	}{Error: errStr})
}
