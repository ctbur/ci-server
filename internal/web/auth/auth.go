package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"
)

type githubUserClient interface {
	GetUser(ctx context.Context, accessToken string) (string, error)
}

type UserAuth struct {
	encryptionKey         []byte
	oauthConfig           *oauth2.Config
	authorizedGitHubUsers map[string]struct{}
}

func NewUserAuth(ghCfg config.GitHubConfig, encryptionKey string) (*UserAuth, error) {
	// Ensure the encryption key is 64 characters (32 bytes) for AES-256
	if len(encryptionKey) != 64 {
		return nil, fmt.Errorf("encryption key must be 64 characters long")
	}
	encryptionKeyBytes, err := hex.DecodeString(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode encryption key: %w", err)
	}

	authorizedUsers := make(map[string]struct{})
	for _, user := range ghCfg.AuthorizedUsers {
		authorizedUsers[user] = struct{}{}
	}

	return &UserAuth{
		encryptionKey: encryptionKeyBytes,
		oauthConfig: &oauth2.Config{
			ClientID:     ghCfg.ClientID,
			ClientSecret: ghCfg.ClientSecret,
			Scopes:       []string{"read:user"},
			Endpoint:     endpoints.GitHub,
		},
		authorizedGitHubUsers: authorizedUsers,
	}, nil
}

func HandleLogin(auth *UserAuth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		oauthState, err := generateOAuthState()
		if err != nil {
			log.ErrorContext(ctx, "Failed to generate OAuth state", slog.Any("error", err))
			http.Error(w, "Failed to initiate OAuth flow", http.StatusInternalServerError)
			return
		}

		encryptedState, err := encrypt(auth.encryptionKey, oauthState)
		if err != nil {
			log.ErrorContext(ctx, "Failed to encrypt OAuth state", slog.Any("error", err))
			http.Error(w, "Failed to initiate OAuth flow", http.StatusInternalServerError)
			return
		}

		// Store in HTTP-only, secure cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "oauth_state",
			Value:    encryptedState,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600, // 10 minutes
		})

		h := sha256.New()
		h.Write([]byte(oauthState.CodeVerifier))
		codeChallenge := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(h.Sum(nil))

		// Build authorization URL with state and PKCE parameters
		authURL := auth.oauthConfig.AuthCodeURL(
			oauthState.State,
			oauth2.AccessTypeOnline,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)

		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	}
}

const sessionDuration = 24 * time.Hour

func HandleCallback(auth *UserAuth, github githubUserClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		cookie, err := r.Cookie("oauth_state")
		if err != nil {
			log.ErrorContext(ctx, "Missing oauth state cookie", slog.Any("error", err))
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}

		// Decrypt and verify state
		oauthState, err := decrypt[OAuthState](auth.encryptionKey, cookie.Value)
		if err != nil {
			log.ErrorContext(ctx, "Failed to decrypt oauth state", slog.Any("error", err))
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		if time.Now().Unix()-oauthState.CreatedAt > 600 {
			log.ErrorContext(ctx, "OAuth state expired")
			http.Error(w, "OAuth flow expired", http.StatusBadRequest)
			return
		}

		// Verify state parameter matches
		state := r.URL.Query().Get("state")
		if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(oauthState.State)) != 1 {
			log.ErrorContext(ctx, "Invalid state parameter")
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}

		// Delete the cookie
		http.SetCookie(w, &http.Cookie{
			Name:   "oauth_state",
			Path:   "/",
			MaxAge: -1,
		})

		// Exchange code for token
		code := r.URL.Query().Get("code")
		if code == "" {
			log.ErrorContext(ctx, "Missing code parameter")
			http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
			return
		}
		token, err := auth.oauthConfig.Exchange(
			r.Context(),
			code,
			oauth2.SetAuthURLParam("code_verifier", oauthState.CodeVerifier),
		)
		if err != nil {
			log.ErrorContext(ctx, "Failed to exchange token", slog.Any("error", err))
			http.Error(w, "OAuth error", http.StatusInternalServerError)
			return
		}

		// Check if GitHub user is authorized
		ghUser, err := github.GetUser(ctx, token.AccessToken)
		if err != nil {
			log.ErrorContext(ctx, "Failed to get GitHub user", slog.Any("error", err))
			http.Error(w, "OAuth error", http.StatusInternalServerError)
			return
		}
		if _, ok := auth.authorizedGitHubUsers[ghUser]; !ok {
			log.ErrorContext(ctx, "Unauthorized GitHub user", slog.String("user", ghUser))
			http.Error(w, "User is not authorized", http.StatusForbidden)
			return
		}

		// Create user session
		userSession := UserSession{
			GitHubUsername: ghUser,
			ExpiresAt:      time.Now().Add(sessionDuration).Unix(),
		}
		encryptedSession, err := encrypt(auth.encryptionKey, userSession)
		if err != nil {
			log.ErrorContext(ctx, "Failed to encrypt user session", slog.Any("error", err))
			http.Error(w, "Error creating session", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "user_session",
			Value:    encryptedSession,
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteStrictMode,
			MaxAge:   int(sessionDuration.Seconds()),
		})

		log.InfoContext(ctx, "User logged in", slog.String("user", ghUser))

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func HandleLogout(auth *UserAuth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		// Delete the user session cookie
		http.SetCookie(w, &http.Cookie{
			Name:   "user_session",
			Path:   "/",
			MaxAge: -1,
		})

		log.InfoContext(ctx, "User logged out")

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

func (a *UserAuth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		cookie, err := r.Cookie("user_session")
		if err != nil {
			if !errors.Is(err, http.ErrNoCookie) {
				log.ErrorContext(ctx, "Failed to read user session cookie", slog.Any("error", err))
			}
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		userSession, err := decrypt[UserSession](a.encryptionKey, cookie.Value)
		if err != nil {
			log.ErrorContext(ctx, "Failed to decrypt user session", slog.Any("error", err))
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		if _, ok := a.authorizedGitHubUsers[userSession.GitHubUsername]; !ok {
			log.WarnContext(ctx, "Unauthorized GitHub user in session", slog.String("user", userSession.GitHubUsername))
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		if time.Now().Unix() > userSession.ExpiresAt {
			log.InfoContext(ctx, "User session expired", slog.String("user", userSession.GitHubUsername))
			http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
			return
		}

		// Add username to context
		log = log.With(slog.String("user", userSession.GitHubUsername))
		ctx = ctxlog.ContextWithLogger(ctx, log)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type OAuthState struct {
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier"`
	CreatedAt    int64  `json:"created_at"`
}

func generateOAuthState() (*OAuthState, error) {
	// Generate a random state
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Generate code verifier and challenge
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	codeVerifier := base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(verifierBytes)

	// Create OAuth state
	oauthState := OAuthState{
		State:        state,
		CodeVerifier: codeVerifier,
		CreatedAt:    time.Now().Unix(),
	}
	return &oauthState, nil
}

type UserSession struct {
	GitHubUsername string `json:"github_username"`
	ExpiresAt      int64  `json:"expires_at"`
}

// Encrypt with AES-GCM which provides both confidentiality and integrity
func encrypt(key []byte, value any) (string, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	cyphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(cyphertext), nil
}

func decrypt[V any](key []byte, encryptedValue string) (V, error) {
	var value V
	ciphertext, err := base64.URLEncoding.DecodeString(encryptedValue)
	if err != nil {
		return value, err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return value, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return value, err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return value, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return value, err
	}

	if err := json.Unmarshal(plaintext, &value); err != nil {
		return value, fmt.Errorf("failed to unmarshal decrypted data: %w", err)
	}

	return value, nil
}
