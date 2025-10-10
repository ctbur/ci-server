package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
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

type ValidationError struct {
	Errors []string
}

func (v *ValidationError) Error() string {
	if len(v.Errors) == 0 {
		return "validation successful"
	}

	return fmt.Sprintf("build metadata validation failed: %s", strings.Join(v.Errors, "; "))
}

var hexRegex = regexp.MustCompile("^[a-fA-F0-9]*$")

// sanitizeBuild sanitizes b to reduce security risks, and returns
// validation errors when the input is not deemed sanitizable.
func sanitizeBuild(b *store.BuildMeta) *ValidationError {
	var validationErrors []string

	// Ref
	if b.Ref == "" {
		validationErrors = append(validationErrors, "Ref (branch/tag reference) is required")
	}
	if !strings.HasPrefix(b.Ref, "refs/") {
		validationErrors = append(validationErrors, "Ref needs to start with 'refs/'")
	}
	if len(b.Ref) > 255 {
		validationErrors = append(validationErrors, "Ref must be fewer than 255 characters")
	}

	// Commit SHA
	if b.CommitSHA == "" {
		validationErrors = append(validationErrors, "Commit SHA is required")
	}
	if !hexRegex.MatchString(b.CommitSHA) {
		validationErrors = append(validationErrors, "Commit SHA must be hex")
	}
	if len(b.CommitSHA) != 40 {
		validationErrors = append(validationErrors, "Commit SHA must be 40 characters long")
	}

	// Author
	if b.Author == "" {
		validationErrors = append(validationErrors, "Author is required")
	}
	if len(b.Author) > 100 {
		validationErrors = append(validationErrors, "Author must be fewer than 100 characters")
	}

	// Message
	if len(b.Message) > 1000 {
		b.Message = b.Message[:1000]
	}

	// Link
	if b.Link != "" && !strings.HasPrefix(b.Link, "https://") {
		validationErrors = append(validationErrors, "Link needs to start with 'https://'")
	}
	if len(b.Link) > 255 {
		validationErrors = append(validationErrors, "Link must be fewer than 256 characters")
	}

	if len(validationErrors) > 0 {
		return &ValidationError{Errors: validationErrors}
	}

	return nil
}

func renderStruct(w http.ResponseWriter, v any, status int) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	return enc.Encode(v)
}
