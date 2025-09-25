package web

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/auth"
	"github.com/ctbur/ci-server/v2/internal/web/ui"
	"github.com/ctbur/ci-server/v2/internal/web/webhook"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, userAuth auth.UserAuth, store store.PGStore, tmpl *template.Template, staticFileDir string) http.Handler {
	mux := http.NewServeMux()

	staticFileServer := http.FileServer(http.Dir(staticFileDir))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFileServer))

	webhookHandler := http.StripPrefix("/webhook", webhook.Handler(cfg, userAuth, store))
	mux.Handle("/webhook/", webhookHandler)

	mux.Handle("/", auth.Middleware(userAuth, ui.Handler(cfg, store, tmpl)))

	return wlog.Middleware(mux)
}

func RunServer(log *slog.Logger, handler http.Handler, port int) error {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	serverErrChan := make(chan error, 1)
	go func() {
		log.Info("Starting server...")
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			serverErrChan <- fmt.Errorf("server error: %w", err)
		}
		serverErrChan <- nil
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait until either a signal is received or the server has an error
	select {
	case <-sigChan:
		log.Info("Received signal, shutting down gracefully...")
	case err := <-serverErrChan:
		if err != nil {
			return fmt.Errorf("server terminated unexpectedly: %w", err)
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("error during server shutdown: %w", err)
	}

	// Wait for the server to shut down
	<-serverErrChan

	log.Info("Server shutdown complete")
	return nil
}
