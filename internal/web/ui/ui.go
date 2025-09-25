package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, s store.PGStore, tmpl *template.Template) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handleBuilds(s, tmpl))
	mux.Handle("GET /builds/{build_id}", handleBuildDetails(s))

	return mux
}

type BuildsPageParams struct {
	Builds []store.BuildWithRepoMeta
}

func handleBuilds(s store.PGStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		builds, err := s.ListBuilds(ctx)
		if err != nil {
			http.Error(w, "Failed to list builds", http.StatusInternalServerError)
			log.Error("Failed to list builds", slog.Any("error", err))
			return
		}

		params := &BuildsPageParams{
			Builds: builds,
		}

		var b bytes.Buffer
		err = tmpl.ExecuteTemplate(&b, "page_builds", params)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			log.Error("Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		b.WriteTo(w)
	}
}

func handleBuildDetails(s store.PGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid build ID", http.StatusNotFound)
			return
		}

		build, err := s.GetBuild(ctx, buildID)
		if err != nil {
			http.Error(w, "Failed to fetch build", http.StatusInternalServerError)
			slog.ErrorContext(ctx, "Failed to fetch build", slog.Any("error", err))
			return
		}

		if build == nil {
			http.Error(w, "Build not found", http.StatusNotFound)
			return
		}

		logs, err := s.GetLogs(ctx, buildID, 0)
		if err != nil {
			http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
			slog.ErrorContext(ctx, "Failed to fetch logs", slog.Any("error", err))
			return
		}

		fmt.Fprintf(w, "Created: %s\n", build.Created.Format("2006-01-02 15:04:05"))
		if build.Started != nil {
			fmt.Fprintf(w, "Started: %s\n", build.Started.Format("2006-01-02 15:04:05"))
		}
		if build.Finished != nil {
			fmt.Fprintf(w, "Started: %s\n", build.Finished.Format("2006-01-02 15:04:05"))
		}
		if build.Result != nil {
			fmt.Fprintf(w, "Result: %s", *build.Result)
		}
		fmt.Fprint(w, "\n")

		for i := range logs {
			w.Write([]byte(logs[i].Text))
			w.Write([]byte("\n"))
		}
	}
}
