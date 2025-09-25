package ui

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg config.Config, s store.PGStore, tmpl *template.Template) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handleBuildList(s, tmpl))
	mux.Handle("GET /builds/{build_id}", handleBuildDetails(s, tmpl))

	return mux
}

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

func handleBuildList(s store.PGStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		builds, err := s.ListBuilds(ctx)
		if err != nil {
			http.Error(w, "Failed to list builds", http.StatusInternalServerError)
			log.Error("Failed to list builds", slog.Any("error", err))
			return
		}

		buildCards := make([]BuildCard, len(builds))
		for i, b := range builds {

			card := BuildCard{
				ID:        b.ID,
				Status:    buildStatus(b.BuildState),
				Message:   b.Message,
				Author:    b.Author,
				Ref:       b.Ref,
				CommitSHA: b.CommitSHA[:7],
				Duration:  durationSinceBuildStart(b.BuildState),
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
		b.WriteTo(w)
	}
}

type LogLine struct {
	Text           string
	TimeSinceStart time.Duration
}

type BuildDetailsPage struct {
	RepoOwner string
	RepoName  string
	Status    string
	Message   string
	Number    uint64
	Started   *time.Time
	Duration  *time.Duration
	LogLines  []LogLine
}

func handleBuildDetails(s store.PGStore, tmpl *template.Template) http.HandlerFunc {
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

		logLines := make([]LogLine, len(logs))
		for i, log := range logs {
			logLines[i] = LogLine{
				Text:           log.Text,
				TimeSinceStart: log.Timestamp.Sub(logs[0].Timestamp),
			}
		}

		params := &BuildDetailsPage{
			RepoOwner: build.Owner,
			RepoName:  build.Name,
			Status:    buildStatus(build.BuildState),
			Message:   build.Message,
			Number:    build.Number,
			Started:   build.Started,
			Duration:  durationSinceBuildStart(build.BuildState),
			LogLines:  logLines,
		}

		var b bytes.Buffer
		err = tmpl.ExecuteTemplate(&b, "page_build_details", params)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			slog.ErrorContext(ctx, "Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		b.WriteTo(w)
	}
}

func durationSinceBuildStart(b store.BuildState) *time.Duration {
	if b.Started == nil {
		return nil
	}

	var duration time.Duration
	if b.Finished != nil {
		duration = b.Finished.Sub(*b.Started)
	} else {
		duration = time.Since(*b.Started)
	}
	return &duration
}

func buildStatus(b store.BuildState) string {
	if b.Result != nil {
		return string(*b.Result)
	} else if b.Started != nil {
		return "running"
	}
	return "pending"
}
