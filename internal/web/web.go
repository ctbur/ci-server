package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/github"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
	"github.com/ctbur/ci-server/v2/internal/web/ui"
	"github.com/ctbur/ci-server/v2/internal/web/webhook"
)

func handler(
	cfg *config.Config,
	githubApp *github.GitHubApp,
	userAuth *auth.UserAuth,
	db *store.DBStore,
	fs *store.FSStore,
	tmpl *template.Template,
) http.Handler {
	mux := http.NewServeMux()

	staticFileServer := http.FileServer(http.FS(ui.StaticFS))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFileServer))

	mux.Handle("GET /auth/login", auth.HandleLogin(userAuth))
	mux.Handle("GET /auth/callback", auth.HandleCallback(userAuth, githubApp))
	mux.Handle("GET /auth/logout", auth.HandleLogout(userAuth))

	mux.Handle("POST /webhook/github", webhook.HandleGitHub(db, cfg))
	if cfg.DevMode {
		mux.Handle("POST /webhook/manual", webhook.HandleManual(db, cfg))
	}

	uiMux := http.NewServeMux()
	uiMux.Handle("GET /{$}", ui.HandleBuildList(db, tmpl))
	uiMux.Handle("GET /hx/builds", ui.HandleBuildListFragment(db, tmpl))
	uiMux.Handle("GET /builds/{build_id}", ui.HandleBuildDetails(db, fs, tmpl))
	uiMux.Handle("GET /hx/builds/{build_id}", ui.HandleBuildDetailsFragment(db, fs, tmpl))

	if cfg.DevMode {
		// Disable auth in dev mode for testing
		mux.Handle("/", uiMux)
	} else {
		mux.Handle("/", userAuth.Middleware(uiMux))
	}

	omitQueryPaths := []string{"/auth/callback"}
	return ctxlog.Middleware(mux, omitQueryPaths)
}

func RunServer(
	ctx context.Context,
	port int,
	cfg *config.Config,
	githubApp *github.GitHubApp,
	auth *auth.UserAuth,
	db *store.DBStore,
	fs *store.FSStore,
) error {
	log := ctxlog.FromContext(ctx)

	tmpl, err := ui.LoadTemplates()
	if err != nil {
		return fmt.Errorf("failed to load templates: %v", err)
	}

	handler := handler(cfg, githubApp, auth, db, fs, tmpl)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,

		ReadTimeout:       1 * time.Second,
		WriteTimeout:      2 * time.Second,
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 1 * time.Second,
	}

	serverErrChan := make(chan error, 1)
	go func() {
		log.Info("Starting server...")
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- fmt.Errorf("server error: %w", err)
		}
		serverErrChan <- nil
	}()

	// Wait until either the context is canceled or the server has an error
	select {
	case <-ctx.Done():
		log.Info("Shutting down server...")
	case err := <-serverErrChan:
		if err != nil {
			return fmt.Errorf("server terminated unexpectedly: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error during server shutdown: %w", err)
	}

	// Wait for the server to shut down
	<-serverErrChan

	log.Info("Server shutdown complete")
	return nil
}
