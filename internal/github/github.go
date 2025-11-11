package github

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

type GitHubApp struct {
	client                  *http.Client
	appID                   uint64
	installationID          uint64
	installationToken       string
	installationTokenExpiry time.Time
	privateKey              *rsa.PrivateKey
}

func NewGitHubApp(
	client *http.Client,
	appID uint64, installationID uint64,
	privateKey *rsa.PrivateKey,
) *GitHubApp {
	return &GitHubApp{
		client:                  client,
		appID:                   appID,
		installationID:          installationID,
		installationToken:       "",
		installationTokenExpiry: time.Time{},
		privateKey:              privateKey,
	}
}

func (a *GitHubApp) issueJWT(t time.Time) (string, error) {
	header := `{"typ":"JWT","alg":"RS256"}`
	claims := fmt.Sprintf(
		`{"iat":%d,"exp":%d,"iss":%d}`,
		t.Add(-time.Minute).Unix(),  // issued 1 minute in the past to allow for clock drift
		t.Add(9*time.Minute).Unix(), // expires in 9 minutes (max lifetime is 10 minutes)
		a.appID,
	)
	payload := fmt.Sprintf(
		"%s.%s",
		base64.RawURLEncoding.EncodeToString([]byte(header)),
		base64.RawURLEncoding.EncodeToString([]byte(claims)),
	)

	if !crypto.SHA256.Available() {
		// TODO: How can we ensure this never happens?
		return "", errors.New("failed to sign JTW: SHA256 hash is unavailable")
	}

	// Hash
	hasher := crypto.SHA256.New()
	_, _ = hasher.Write([]byte(payload)) // Does not return an error
	hashed := hasher.Sum(nil)

	// Sign
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.privateKey, crypto.SHA256, hashed)
	if err != nil {
		return "", fmt.Errorf("failed to sign JTW: %w", err)
	}

	return payload + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (a *GitHubApp) getInstallationToken(ctx context.Context) (string, time.Time, error) {
	if a.installationToken != "" || time.Until(a.installationTokenExpiry) < 2*time.Minute {
		token, expiry, err := a.refreshInstallationToken(ctx)
		if err != nil {
			return "", time.Time{}, fmt.Errorf("failed to refresh installation token: %w", err)
		}
		a.installationToken = token
		a.installationTokenExpiry = expiry
	}

	return a.installationToken, a.installationTokenExpiry, nil
}

func (a *GitHubApp) refreshInstallationToken(ctx context.Context) (string, time.Time, error) {
	// Create request
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", a.installationID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	jwt, err := a.issueJWT(time.Now())
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to issue JWT: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	// Perform request
	resp, err := a.client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", time.Time{}, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse response
	var result struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", time.Time{}, fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Token, result.ExpiresAt, nil
}

type CommitState string

const (
	CommitStateError   CommitState = "error"
	CommitStateFailure CommitState = "failure"
	CommitStatePending CommitState = "pending"
	CommitStateSuccess CommitState = "success"
)

func (a *GitHubApp) CreateCommitStatus(
	ctx context.Context,
	owner, repo, sha string,
	state CommitState,
	description string,
	targetURL string,
	contextStr string,
) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/statuses/%s", owner, repo, sha)
	payloadBytes, err := json.Marshal(map[string]string{
		"state":       string(state),
		"description": description,
		"target_url":  targetURL,
		"context":     contextStr,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	log := wlog.FromContext(ctx)
	log.DebugContext(ctx,
		"CreateCommitStatus",
		slog.String("client", "github"),
		slog.String("payload", string(payloadBytes)),
	)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	token, _, err := a.getInstallationToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get installation token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}
