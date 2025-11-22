package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/store"
)

func HandleGitHub(b BuildCreator, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := ctxlog.FromContext(r.Context())

		ctx := r.Context()

		// Only process push events
		if r.Header.Get("X-GitHub-Event") != "push" {
			w.WriteHeader(http.StatusOK)
			return
		}

		payload, err := io.ReadAll(r.Body)
		if err != nil {
			log.ErrorContext(ctx, "Failed to read request body", slog.Any("error", err))
			errMsg := fmt.Sprintf("Failed to read request body: %v", err)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		// Verify signature
		if cfg.GitHub.WebhookSecret == "" {
			log.ErrorContext(ctx, "No webhook secret configured")
			http.Error(w, "No webhook secret configured", http.StatusInternalServerError)
			return
		}

		expectedSignature := r.Header.Get("X-Hub-Signature-256")
		if len(expectedSignature) == 0 {
			log.ErrorContext(ctx, "Missing X-Hub-Signature-256 header")
			http.Error(w, "Missing X-Hub-Signature-256 header", http.StatusUnauthorized)
			return
		}
		expectedSignature = strings.TrimPrefix(expectedSignature, "sha256=")

		mac := hmac.New(sha256.New, []byte(cfg.GitHub.WebhookSecret))
		_, _ = mac.Write(payload)
		calculatedSignature := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(expectedSignature), []byte(calculatedSignature)) {
			log.ErrorContext(ctx, "Invalid signature")
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}

		// Unmarshal
		var event *PushEvent
		err = json.Unmarshal(payload, &event)
		if err != nil {
			log.ErrorContext(ctx, "Failed to unmarshal JSON", slog.Any("error", err))
			errMsg := fmt.Sprintf("Failed to unmarshal JSON: %v", err)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		// Validate installation ID is authorized
		installationID := event.Installation.ID
		if !slices.Contains(cfg.GitHub.AuthorizedInstallationIDs, installationID) {
			log.ErrorContext(ctx,
				"Unauthorized installation ID",
				slog.Uint64("installation_id", installationID),
			)
			errMsg := fmt.Sprintf("Unauthorized installation ID: %d", installationID)
			http.Error(w, errMsg, http.StatusForbidden)
			return
		}

		// Obtain config for the target repository
		owner := event.Repo.Owner.Login
		if owner == "" {
			log.ErrorContext(ctx, "Missing repository owner in payload")
			http.Error(w, "Missing repository owner in payload", http.StatusBadRequest)
			return
		}

		name := event.Repo.Name
		if name == "" {
			log.ErrorContext(ctx, "Missing repository name in payload")
			http.Error(w, "Missing repository name in payload", http.StatusBadRequest)
			return
		}

		repoCfg := cfg.Repos.Get(owner, name)
		if repoCfg == nil {
			log.ErrorContext(ctx,
				"Repository not configured",
				slog.String("owner", owner),
				slog.String("name", name),
			)
			errMsg := fmt.Sprintf("Repository %s/%s not configured", owner, name)
			http.Error(w, errMsg, http.StatusNotFound)
			return
		}

		// Ignore events with no head commit (e.g. branch deletions)
		if event.HeadCommit == nil {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Create new build
		build := store.BuildMeta{
			Link:      event.HeadCommit.URL,
			Ref:       event.Ref,
			CommitSHA: event.HeadCommit.ID,
			Message:   event.HeadCommit.Message,
			Author:    event.HeadCommit.Author.Username,
		}

		err = sanitizeBuild(&build)
		if err != nil {
			log.ErrorContext(ctx, "Invalid build", slog.Any("error", err))
			http.Error(w, fmt.Sprintf("Invalid build: %v", err), http.StatusBadRequest)
			return
		}

		buildID, err := b.CreateBuild(ctx, owner, name, build, time.Now())
		if err != nil {
			log.ErrorContext(ctx, "Failed to create build", slog.Any("error", err))
			http.Error(w, "Failed to create build", http.StatusInternalServerError)
			return
		}

		log.InfoContext(ctx, "Build created via GitHub webhook", slog.Uint64("id", buildID))
		w.WriteHeader(http.StatusOK)
	}
}

type User struct {
	Login     string `json:"login"`
	AvatarURL string `json:"avatar_url"`
}

type CommitAuthor struct {
	Username string `json:"username"`
}

type HeadCommit struct {
	ID      string       `json:"id"`
	Message string       `json:"message"`
	Author  CommitAuthor `json:"author"`
	URL     string       `json:"url"`
}

type PushEventRepository struct {
	Name  string `json:"name"`
	Owner User   `json:"owner"`
}

type Installation struct {
	ID uint64 `json:"id"`
}

type PushEvent struct {
	Ref          string              `json:"ref"`
	Repo         PushEventRepository `json:"repository"`
	HeadCommit   *HeadCommit         `json:"head_commit"`
	Installation Installation        `json:"installation"`
}
