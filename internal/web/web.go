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
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
	"github.com/ctbur/ci-server/v2/internal/web/ui"
	"github.com/ctbur/ci-server/v2/internal/web/webhook"
)

func Handler(
	cfg *config.Config,
	userAuth auth.UserAuth,
	db *store.DBStore,
	fs *store.FSStore,
	tmpl *template.Template,
	staticFileDir string,
) http.Handler {
	mux := http.NewServeMux()

	staticFileServer := http.FileServer(http.Dir(staticFileDir))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFileServer))

	mux.Handle("POST /webhook/manual", userAuth.Middleware(webhook.HandleManual(db, cfg)))
	mux.Handle("POST /webhook/github", webhook.HandleGitHub(db, cfg))

	uiMux := http.NewServeMux()
	uiMux.Handle("GET /{$}", ui.HandleBuildList(db, tmpl))
	uiMux.Handle("GET /hx/builds", ui.HandleBuildListFragment(db, tmpl))
	uiMux.Handle("GET /builds/{build_id}", ui.HandleBuildDetails(db, fs, tmpl))
	uiMux.Handle("GET /hx/builds/{build_id}", ui.HandleBuildDetailsFragment(db, fs, tmpl))
	mux.Handle("/", userAuth.Middleware(uiMux))

	return ctxlog.Middleware(mux)
}

func RunServer(ctx context.Context, handler http.Handler, port int) error {
	log := ctxlog.FromContext(ctx)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,

		ReadTimeout:       1 * time.Second,
		WriteTimeout:      300 * time.Second, // Very long timeout for SSE
		IdleTimeout:       30 * time.Second,
		ReadHeaderTimeout: 2 * time.Second,
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
