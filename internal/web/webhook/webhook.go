package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
)

type BuildCreator interface {
	CreateBuild(ctx context.Context, repoOwner, repoName string, build store.BuildMeta, ts time.Time) (uint64, error)
}

func Handler(cfg *config.Config, userAuth auth.UserAuth, b BuildCreator) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("POST /manual", userAuth.Middleware(handleManual(b, cfg)))
	mux.Handle("POST /github", handleGitHub(b, cfg))

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

func renderStruct(w http.ResponseWriter, v any, status int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}
