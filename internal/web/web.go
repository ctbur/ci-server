package web

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/api"
	"github.com/ctbur/ci-server/v2/internal/web/ui"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, store store.PGStore, bld build.Builder) http.Handler {
	mux := http.NewServeMux()

	apiHandler := http.StripPrefix("/api", api.Handler(cfg, store, bld))
	mux.Handle("/api/", apiHandler)

	uiHandler := http.StripPrefix("/ui", ui.Handler(cfg, store))
	mux.Handle("/ui/", uiHandler)

	return wlog.Middleware(mux)
}

func RunServer(log *slog.Logger, handler http.Handler, port int) error {
	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: handler,
	}

	serverErrChan := make(chan error, 1)
	go func() {
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
