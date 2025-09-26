package webhook

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

type ManualPayload struct {
	Owner     string `json:"owner"`
	Name      string `json:"name"`
	Link      string `json:"link"`
	Ref       string `json:"ref"`
	CommitSHA string `json:"commit_sha"`
	Message   string `json:"message"`
	Author    string `json:"author"`
}

type ManualResult struct {
	BuildID uint64 `json:"build_id"`
}

func handleManual(
	s BuildCreationStore,
	cfg *config.Config,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := wlog.FromContext(r.Context())
		ctx := r.Context()

		payload, err := decodeJSON[ManualPayload](r.Body)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to decode JSON: %v", err)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		repoCfg := cfg.GetRepoConfig(payload.Owner, payload.Name)
		if repoCfg == nil {
			http.Error(w, "Repository not configured", http.StatusNotFound)
			return
		}

		repo, err := s.GetRepo(ctx, payload.Owner, payload.Name)
		if err != nil {
			http.Error(w, "Failed to query repository data", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to query repository data", slog.Any("error", err))
			return
		}

		buildNumber, err := s.IncrementBuildCounter(ctx, repo.ID)
		if err != nil {
			http.Error(w, "Failed to increment build counter", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to increment build counter", slog.Any("error", err))
			return
		}

		build := store.BuildMeta{
			RepoID:    repo.ID,
			Number:    buildNumber,
			Link:      payload.Link,
			Ref:       payload.Ref,
			CommitSHA: payload.CommitSHA,
			Message:   payload.Message,
			Author:    payload.Author,
		}

		buildID, err := s.CreateBuild(ctx, build)
		if err != nil {
			http.Error(w, "Failed to create build", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to create build", slog.Any("error", err))
			return
		}

		log.InfoContext(ctx, "Build created via manual webhook", slog.Uint64("id", buildID))
		res := ManualResult{
			BuildID: buildID,
		}
		renderStruct(w, res, http.StatusOK)
	}
}
