package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

func Handler(cfg config.Config, s store.PGStore) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handleBuilds(s))
	mux.Handle("GET /builds/{build_id}", handleBuildDetails(s))

	return mux
}

func handleBuilds(s store.PGStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

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
			http.Error(w, fmt.Sprintf("Unable to fetch logs: %v", err), http.StatusNotFound)
			return
		}

		for i := range logs {
			w.Write([]byte(logs[i].Text))
			w.Write([]byte("\n"))
		}
	}
}
