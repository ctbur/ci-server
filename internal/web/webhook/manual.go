package webhook

import (
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/store"
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

func HandleManual(b BuildCreator, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := ctxlog.FromContext(r.Context())
		ctx := r.Context()

		payload, err := decodeJSON[ManualPayload](r.Body)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to decode JSON: %v", err)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		repoCfg := cfg.Repos.Get(payload.Owner, payload.Name)
		if repoCfg == nil {
			http.Error(w, "Repository not configured", http.StatusNotFound)
			return
		}

		build := store.BuildMeta{
			Link:      payload.Link,
			Ref:       payload.Ref,
			CommitSHA: payload.CommitSHA,
			Message:   payload.Message,
			Author:    payload.Author,
		}

		err = sanitizeBuild(&build)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid build: %v", err), http.StatusBadRequest)
			return
		}

		buildID, err := b.CreateBuild(ctx, payload.Owner, payload.Name, build, time.Now())
		if err != nil {
			http.Error(w, "Failed to create build", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to create build", slog.Any("error", err))
			return
		}

		log.InfoContext(ctx, "Build created via manual webhook", slog.Uint64("id", buildID))
		res := ManualResult{
			BuildID: buildID,
		}
		_ = renderStruct(w, res, http.StatusOK)
	}
}
