package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
)

func Handler(cfg config.Config, pgStore store.PGStore) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /builds/{build_id}", handleBuilds(pgStore.Repo, pgStore.Build, pgStore.Log))

	return mux
}

func handleBuilds(
	repoStore store.RepoStore,
	buildStore store.BuildStore,
	logStore store.LogStore,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid build ID", http.StatusNotFound)
			return
		}

		logs, err := logStore.Get(r.Context(), buildID, 0)
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
