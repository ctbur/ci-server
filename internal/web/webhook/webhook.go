package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
)

type BuildCreationStore interface {
	GetRepo(ctx context.Context, owner, name string) (*store.Repo, error)
	IncrementBuildCounter(ctx context.Context, repoID uint64) (uint64, error)
	CreateBuild(ctx context.Context, build store.BuildMeta) (uint64, error)
}

func Handler(cfg *config.Config, userAuth auth.UserAuth, s BuildCreationStore) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /manual", userAuth.Middleware(handleManual(s, cfg)))
	mux.Handle("POST /github", handleGitHub(s, cfg))

	return mux
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
