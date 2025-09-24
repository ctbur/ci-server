package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, s store.PGStore) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /webhook", handleWebhook(s, cfg))

	return mux
}

type WebhookPayload struct {
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	CommitSHA string `json:"commit_sha"`
}

type WebhookResult struct {
	BuildID uint64 `json:"build_id"`
}

func handleWebhook(
	s store.PGStore,
	cfg config.Config,
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
		repo, err := s.GetRepo(r.Context(), payload.Owner, payload.Name)
		if err != nil {
			renderError(
				w,
				fmt.Errorf("Failed to find repository %s/%s in database", payload.Owner, payload.Name),
				http.StatusNotFound,
			)
			return
		}

		buildNumber, err := s.IncrementBuildCounter(r.Context(), repo.ID)
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
		buildID, err := s.CreateBuild(r.Context(), build)
		if err != nil {
			renderError(w, err, http.StatusInternalServerError)
			return
		}

		res := WebhookResult{
			BuildID: buildID,
		}
		renderStruct(w, res, http.StatusOK)
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
