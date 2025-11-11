package ui

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

type BuildListPage struct {
	BuildCards []BuildCard
}

type BuildCard struct {
	ID        uint64
	Status    string
	Message   string
	Author    string
	Ref       string
	CommitSHA string
	Duration  *time.Duration
	Started   *time.Time
}

func HandleBuildList(db *store.DBStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		builds, err := db.ListBuilds(ctx)
		if err != nil {
			http.Error(w, "Failed to list builds", http.StatusInternalServerError)
			log.Error("Failed to list builds", slog.Any("error", err))
			return
		}

		buildCards := make([]BuildCard, len(builds))
		for i, b := range builds {
			card := BuildCard{
				ID:        b.ID,
				Status:    buildStatus(b),
				Message:   shortCommitMessage(b.Message),
				Author:    b.Author,
				Ref:       b.Ref,
				CommitSHA: b.CommitSHA[:min(7, len(b.CommitSHA))],
				Duration:  durationSinceBuildStart(b),
				Started:   b.Started,
			}
			buildCards[i] = card
		}
		params := &BuildListPage{
			BuildCards: buildCards,
		}

		var b bytes.Buffer
		err = tmpl.ExecuteTemplate(&b, "page_build_list", params)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			log.Error("Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = b.WriteTo(w)
	}
}
