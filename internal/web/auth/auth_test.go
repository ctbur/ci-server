package auth

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/config"
)

type mockGitHubApp struct {
	username string
	err      error
}

func (m *mockGitHubApp) GetUser(ctx context.Context, accessToken string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.username, nil
}

func TestEncryptDecrypt(t *testing.T) {
	key, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	original := UserSession{
		GitHubUsername: "testuser",
		ExpiresAt:      time.Now().Unix(),
	}

	encrypted, err := encrypt(key, original)
	assert.NoError(t, err, "encrypt() should not error")

	decrypted, err := decrypt[UserSession](key, encrypted)
	assert.NoError(t, err, "decrypt() should not error")
	assert.Equal(t, decrypted, original, "Decrypted and original mismatch")
}

func TestGenerateOAuthState(t *testing.T) {
	state1, err := generateOAuthState()
	assert.NoError(t, err, "generateOAuthState() should not error")

	state2, err := generateOAuthState()
	assert.NoError(t, err, "generateOAuthState() should not error")

	if state1.State == state2.State {
		t.Error("states should be unique")
	}
	if state1.CodeVerifier == state2.CodeVerifier {
		t.Error("code verifiers should be unique")
	}
}

func TestMiddleware(t *testing.T) {
	cfg := config.GitHubConfig{
		ClientID:        "test-client-id",
		ClientSecret:    "test-client-secret",
		AuthorizedUsers: []string{"user1"},
	}
	auth, _ := NewUserAuth(cfg, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Helper to create session cookies
	createSessionCookie := func(username string, expiresIn time.Duration) *http.Cookie {
		session := UserSession{
			GitHubUsername: username,
			ExpiresAt:      time.Now().Add(expiresIn).Unix(),
		}
		encrypted, _ := encrypt(auth.encryptionKey, session)
		return &http.Cookie{
			Name:  "user_session",
			Value: encrypted,
		}
	}

	tests := []struct {
		name         string
		cookie       *http.Cookie
		wantStatus   int
		wantLocation string
	}{
		{
			name:         "no session cookie redirects to login",
			cookie:       nil,
			wantStatus:   http.StatusSeeOther,
			wantLocation: "/auth/login",
		},
		{
			name:       "valid session allows access",
			cookie:     createSessionCookie("user1", 1*time.Hour),
			wantStatus: http.StatusOK,
		},
		{
			name:         "expired session redirects to login",
			cookie:       createSessionCookie("user1", -1*time.Hour),
			wantStatus:   http.StatusSeeOther,
			wantLocation: "/auth/login",
		},
		{
			name: "invalid session cookie redirects to login",
			cookie: &http.Cookie{
				Name:  "user_session",
				Value: "invalid-encrypted-data",
			},
			wantStatus:   http.StatusSeeOther,
			wantLocation: "/auth/login",
		},
		{
			name:         "valid session with unauthorized user redirects to login",
			cookie:       createSessionCookie("unauthorized-user", 1*time.Hour),
			wantStatus:   http.StatusSeeOther,
			wantLocation: "/auth/login",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			if tt.cookie != nil {
				req.AddCookie(tt.cookie)
			}
			w := httptest.NewRecorder()

			auth.Middleware(nextHandler).ServeHTTP(w, req)

			resp := w.Result()
			if resp.StatusCode != tt.wantStatus {
				t.Errorf("got status %d, want %d", resp.StatusCode, tt.wantStatus)
			}

			if tt.wantLocation != "" {
				location := resp.Header.Get("Location")
				if location != tt.wantLocation {
					t.Errorf("got location %s, want %s", location, tt.wantLocation)
				}
			}
		})
	}
}

func TestHandleCallback_AuthorizedUser(t *testing.T) {
	cfg := config.GitHubConfig{
		ClientID:        "test-client-id",
		ClientSecret:    "test-client-secret",
		AuthorizedUsers: []string{"authorized-user"},
	}
	auth, _ := NewUserAuth(cfg, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	// Mock OAuth server
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login/oauth/access_token" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"access_token": "mock-access-token",
				"token_type":   "bearer",
			})
		}
	}))
	defer oauthServer.Close()

	auth.oauthConfig.Endpoint.TokenURL = oauthServer.URL + "/login/oauth/access_token"

	tests := []struct {
		name         string
		username     string
		wantStatus   int
		wantRedirect string
	}{
		{
			name:         "authorized user",
			username:     "authorized-user",
			wantStatus:   http.StatusSeeOther,
			wantRedirect: "/",
		},
		{
			name:       "unauthorized user",
			username:   "unauthorized-user",
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockGH := &mockGitHubApp{username: tt.username}
			handler := HandleCallback(auth, mockGH)

			oauthState := OAuthState{
				State:        "test-state",
				CodeVerifier: "test-verifier",
				CreatedAt:    time.Now().Unix(),
			}
			encryptedState, _ := encrypt(auth.encryptionKey, oauthState)

			params := url.Values{}
			params.Add("code", "test-code")
			params.Add("state", "test-state")

			req := httptest.NewRequest("GET", "/auth/callback?"+params.Encode(), nil)
			req.AddCookie(&http.Cookie{
				Name:  "oauth_state",
				Value: encryptedState,
			})
			w := httptest.NewRecorder()

			handler.ServeHTTP(w, req)

			resp := w.Result()
			assert.Equal(t, resp.StatusCode, tt.wantStatus, "incorrect status code")

			if tt.wantRedirect != "" {
				assert.Equal(t, resp.Header.Get("Location"), tt.wantRedirect, "incorrect redirect location")
			}
		})
	}
}
