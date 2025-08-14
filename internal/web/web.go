package web

import (
	"net/http"

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
