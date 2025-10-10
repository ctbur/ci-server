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
	"strings"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func handleGitHub(b BuildCreator, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := wlog.FromContext(r.Context())
		ctx := r.Context()

		// Only process push events
		if r.Header.Get("X-GitHub-Event") != "push" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Read payload and unmarshal
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to read request body: %v", err)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		var event *PushEvent
		err = json.Unmarshal(payload, &event)
		if err != nil {
			errMsg := fmt.Sprintf("Failed to unmarshal JSON: %v", err)
			http.Error(w, errMsg, http.StatusBadRequest)
			return
		}

		// Obtain config for the target repository
		owner := event.Repo.Owner.Login
		if owner == "" {
			http.Error(w, "Missing repository owner in payload", http.StatusBadRequest)
			return
		}

		name := event.Repo.Name
		if name == "" {
			http.Error(w, "Missing repository name in payload", http.StatusBadRequest)
			return
		}

		repoCfg := cfg.Repos.Get(owner, name)
		if repoCfg == nil {
			errMsg := fmt.Sprintf("Repository %s/%s not configured", owner, name)
			http.Error(w, errMsg, http.StatusNotFound)
			return
		}

		// Verify signature
		// We can only do it after unmarshalling the JSON because we need to
		// know which webhook secret to use.
		if repoCfg.WebhookSecret == nil {
			errMsg := fmt.Sprintf("Repositoroy %s/%s has no webhook secret configured", owner, name)
			http.Error(w, errMsg, http.StatusInternalServerError)
			return
		}

		signature := r.Header.Get("X-Hub-Signature-256")
		if len(signature) == 0 {
			http.Error(w, "Missing X-Hub-Signature-256 header", http.StatusUnauthorized)
			return
		}
		signature = strings.TrimPrefix(signature, "sha256=")

		mac := hmac.New(sha256.New, []byte(*repoCfg.WebhookSecret))
		_, _ = mac.Write(payload)
		expectedMAC := hex.EncodeToString(mac.Sum(nil))

		if !hmac.Equal([]byte(signature), []byte(expectedMAC)) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
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
			http.Error(w, fmt.Sprintf("Invalid build: %v", err), http.StatusBadRequest)
			return
		}

		buildID, err := b.CreateBuild(ctx, owner, name, build, time.Now())
		if err != nil {
			http.Error(w, "Failed to create build", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to create build", slog.Any("error", err))
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

type PushEvent struct {
	Ref        string              `json:"ref"`
	Repo       PushEventRepository `json:"repository"`
	HeadCommit *HeadCommit         `json:"head_commit"`
}
