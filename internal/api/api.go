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
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/go-chi/chi/v5"
)

type API struct {
}

func New() API {
	return API{}
}

func (a API) Handler(store store.PGStore, cfg config.Config, bld build.Builder) http.Handler {
	r := chi.NewRouter()
	r.Use(loggerMiddleware)

	r.Route("/webhook", func(r chi.Router) {
		r.Post("/", handleWebhook(store.Repo, store.Build, store.Log, cfg, bld))
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

func handleWebhook(
	repoStore store.RepoStore,
	buildStore store.BuildStore,
	logStore store.LogStore,
	cfg config.Config,
	bld build.Builder,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := LoggerFromContext(r.Context())
		log.Info("HandleWebhook called")

		repo := store.RepoMeta{
			Owner: "godotengine",
			Name:  "godot",
		}
		repoID, err := repoStore.Create(r.Context(), repo)
		if err != nil {
			renderError(w, err, 500)
			// TODO: don't create the repo here
			//return
		}

		buildNumber, err := repoStore.IncrementBuildCounter(r.Context(), repoID)

		build := store.BuildMeta{
			RepoID:    repoID,
			Number:    buildNumber,
			Link:      "https://github.com/godotengine/godot/commit/bbf29a537f3d2875bba4304b1543d4bf0278b6d9",
			Ref:       "refs/heads/branch",
			CommitSHA: "bbf29a537f3d2875bba4304b1543d4bf0278b6d9",
			Message:   "Build message",
			Author:    "some author",
		}
		buildID, err := buildStore.Create(r.Context(), build)
		if err != nil {
			renderError(w, err, 500)
			return
		}

		result, err := bld.Build(
			logStore,
			&repo, &build, buildID,
			[]string{"scons", "platform=linux"},
		)
		if result != nil {
			log.Info(
				"Build completed",
				slog.String("status", string(result.Status)),
			)
		} else {
			log.Error("Build failed: %w", slog.Any("error", err))
		}

		if err != nil {
			renderError(w, err, 500)
			return
		}

		renderStruct(w, struct{}{}, 200)
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
