package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, pgStore store.PGStore, bld build.Builder) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /webhook", handleWebhook(pgStore.Repo, pgStore.Build, pgStore.Log, cfg, bld))

	return mux
}

func handleWebhook(
	repoStore store.RepoStore,
	buildStore store.BuildStore,
	logStore store.LogStore,
	cfg config.Config,
	bld build.Builder,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := wlog.FromContext(r.Context())
		log.Info("HandleWebhook called")

		repoCfg := cfg.GetRepoConfig("godotengine", "godot")
		repo, err := repoStore.Get(r.Context(), "godotengine", "godot")

		buildNumber, err := repoStore.IncrementBuildCounter(r.Context(), repo.ID)
		if err != nil {
			renderError(w, err, 500)
			return
		}

		build := store.BuildMeta{
			RepoID:    repo.ID,
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
			&repo.RepoMeta, &build, buildID,
			repoCfg.BuildCommand,
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
