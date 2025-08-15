package ui

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, s store.PGStore) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handleBuilds(s))
	mux.Handle("GET /builds/{build_id}", handleBuildDetails(s))

	return mux
}

func handleBuilds(s store.PGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		builds, err := s.ListBuildsWithRepo(ctx)
		if err != nil {
			http.Error(w, "Failed to list builds", http.StatusNotFound)
			log.Error("Failed to list builds", slog.Any("error", err))
			return
		}

		for i := range builds {
			b := &builds[i]

			finishedStr := "TBD"
			if b.Finished != nil {
				finishedStr = b.Finished.Format("2006-01-02T15:04:05.000Z")
			}

			line := fmt.Sprintf(
				"%s/%s Nr. %d: %s - %s",
				b.RepoMeta.Owner,
				b.RepoMeta.Name,
				b.Number,
				b.Created.Format("2006-01-02T15:04:05.000Z"),
				finishedStr,
			)
			w.Write([]byte(line))
			w.Write([]byte("\n"))
		}
	}
}

func handleBuildDetails(s store.PGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid build ID", http.StatusNotFound)
			return
		}

		logs, err := s.GetLogs(r.Context(), buildID, 0)
		if err != nil {
			http.Error(w, "Failed to fetch logs", http.StatusNotFound)
			slog.Error("Failed to fetch logs", slog.Any("error", err))
			return
		}

		for i := range logs {
			w.Write([]byte(logs[i].Text))
			w.Write([]byte("\n"))
		}
	}
}
