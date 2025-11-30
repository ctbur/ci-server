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

	"github.com/ctbur/ci-server/v2/internal/ctxlog"
)

type GitHubApp struct {
	client             *http.Client
	privateKey         *rsa.PrivateKey
	appID              ApplicationID
	appToken           string
	appTokenExpiry     time.Time
	installationTokens map[InstallationID]struct {
		token  string
		expiry time.Time
	}
}

type ApplicationID uint64

type InstallationID uint64

func NewGitHubApp(
	client *http.Client, privateKey *rsa.PrivateKey, appID ApplicationID,
) *GitHubApp {
	return &GitHubApp{
		client:     client,
		privateKey: privateKey,
		appID:      appID,
		installationTokens: make(map[InstallationID]struct {
			token  string
			expiry time.Time
		}),
	}
}

func (a *GitHubApp) GetUser(ctx context.Context, accessToken string) (string, error) {
	log := ctxlog.FromContext(ctx)
	log.DebugContext(ctx,
		"GetUser",
		slog.String("client", "github"),
	)

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/vnd.github+json")

	// Perform request
	resp, err := a.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to perform request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse response
	var result struct {
		Login string `json:"login"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	return result.Login, nil
}

func (a *GitHubApp) getAppToken(ctx context.Context) (string, error) {
	if a.appToken == "" || time.Until(a.appTokenExpiry) < 2*time.Minute {
		token, expiry, err := a.issueAppToken(time.Now())
		if err != nil {
			return "", fmt.Errorf("failed to issue app token: %w", err)
		}
		a.appToken = token
		a.appTokenExpiry = expiry

		log := ctxlog.FromContext(ctx)
		log.InfoContext(ctx,
			"Issued new GitHub App token",
			slog.String("client", "github"),
			slog.Time("expiry", expiry),
		)
	}

	return a.appToken, nil
}

func (a *GitHubApp) issueAppToken(now time.Time) (string, time.Time, error) {
	expiresAt := now.Add(9 * time.Minute)
	header := `{"typ":"JWT","alg":"RS256"}`
	claims := fmt.Sprintf(
		`{"iat":%d,"exp":%d,"iss":%d}`,
		now.Add(-time.Minute).Unix(), // issued 1 minute in the past to allow for clock drift
		expiresAt.Unix(),             // expires in 9 minutes (max lifetime is 10 minutes)
		a.appID,
	)
	payload := fmt.Sprintf(
		"%s.%s",
		base64.RawURLEncoding.EncodeToString([]byte(header)),
		base64.RawURLEncoding.EncodeToString([]byte(claims)),
	)

	if !crypto.SHA256.Available() {
		// TODO: How can we ensure this never happens?
		return "", time.Time{}, errors.New("failed to sign JTW: SHA256 hash is unavailable")
	}

	// Hash
	hasher := crypto.SHA256.New()
	_, _ = hasher.Write([]byte(payload)) // Does not return an error
	hashed := hasher.Sum(nil)

	// Sign
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.privateKey, crypto.SHA256, hashed)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to sign JTW: %w", err)
	}

	return payload + "." + base64.RawURLEncoding.EncodeToString(sig), expiresAt, nil
}

func (a *GitHubApp) GetInstallationToken(ctx context.Context, installation InstallationID) (string, error) {
	t, exist := a.installationTokens[installation]

	if !exist || time.Until(t.expiry) < 2*time.Minute {
		token, expiry, err := a.refreshInstallationToken(ctx, installation)
		if err != nil {
			return "", fmt.Errorf("failed to refresh installation token: %w", err)
		}
		t.token = token
		t.expiry = expiry
		a.installationTokens[installation] = t
	}

	return t.token, nil
}

func (a *GitHubApp) refreshInstallationToken(
	ctx context.Context,
	installation InstallationID,
) (string, time.Time, error) {
	// Create request
	url := fmt.Sprintf("https://api.github.com/app/installations/%d/access_tokens", installation)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to create request: %w", err)
	}

	appToken, err := a.getAppToken(ctx)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("failed to issue app token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+appToken)
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

	log := ctxlog.FromContext(ctx)
	log.InfoContext(ctx,
		"Issued new GitHub Installation token",
		slog.String("client", "github"),
		slog.Uint64("installation_id", uint64(installation)),
		slog.Time("expiry", result.ExpiresAt),
	)

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
	installation InstallationID,
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

	log := ctxlog.FromContext(ctx)
	log.DebugContext(ctx,
		"CreateCommitStatus",
		slog.String("client", "github"),
		slog.String("payload", string(payloadBytes)),
	)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	token, err := a.GetInstallationToken(ctx, installation)
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
