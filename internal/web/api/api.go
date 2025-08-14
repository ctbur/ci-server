package api

import (
	"encoding/json"
	"fmt"
	"io"
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

type WebhookPayload struct {
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
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

		payload, err := decodeJSON[WebhookPayload](r.Body)
		if err != nil {
			renderError(w, err, http.StatusBadRequest)
			return
		}

		repoCfg := cfg.GetRepoConfig(payload.Owner, payload.Name)
		if repoCfg == nil {
			renderError(
				w,
				fmt.Errorf("Repository %s/%s is not configured", payload.Owner, payload.Name),
				http.StatusNotFound,
			)
			return
		}
		repo, err := repoStore.Get(r.Context(), payload.Owner, payload.Name)
		if err != nil {
			renderError(
				w,
				fmt.Errorf("Failed to find repository %s/%s in database", payload.Owner, payload.Name),
				http.StatusNotFound,
			)
			return
		}

		buildNumber, err := repoStore.IncrementBuildCounter(r.Context(), repo.ID)
		if err != nil {
			renderError(w, err, http.StatusInternalServerError)
			return
		}

		build := store.BuildMeta{
			RepoID:    repo.ID,
			Number:    buildNumber,
			Link:      "some link",
			Ref:       "refs/heads/branch",
			CommitSHA: payload.CommitSHA,
			Message:   "Build message",
			Author:    "some author",
		}
		buildID, err := buildStore.Create(r.Context(), build)
		if err != nil {
			renderError(w, err, http.StatusInternalServerError)
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
			log.Error("Build failed", slog.Any("error", err))
		}

		if err != nil {
			renderError(w, err, http.StatusInternalServerError)
			return
		}

		renderStruct(w, struct{}{}, http.StatusOK)
	}
}

func decodeJSON[T any](body io.Reader) (*T, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	var v T
	if err := decoder.Decode(&v); err != nil {
		return nil, fmt.Errorf("failed to decode JSON: %w", err)
	}

	return &v, nil
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
