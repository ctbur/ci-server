package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ctbur/ci-server/v2/internal/config"
	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

func Handler(cfg *config.Config, s store.PGStore, l store.LogStore, tmpl *template.Template) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /{$}", handleBuildList(s, tmpl))
	mux.Handle("GET /builds/{build_id}", handleBuildDetails(s, l, tmpl))
	mux.Handle("GET /builds/{build_id}/log-stream", handleLogStream(s, l, tmpl))

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

type LogLine struct {
	Number         uint
	Text           string
	TimeSinceStart time.Duration
}

type BuildDetailsPage struct {
	ID        uint64
	RepoOwner string
	RepoName  string
	Status    string
	Message   string
	Number    uint64
	Started   *time.Time
	Duration  *time.Duration
	LogLines  []LogLine
}

func handleBuildDetails(s store.PGStore, l store.LogStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid build ID", http.StatusNotFound)
			return
		}

		build, err := s.GetBuild(ctx, buildID)
		if err != nil {
			http.Error(w, "Failed to fetch build", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to fetch build", slog.Any("error", err))
			return
		}

		if build == nil {
			http.Error(w, "Build not found", http.StatusNotFound)
			return
		}

		var logLines []LogLine
		if build.Started != nil {
			logs, err := l.GetLogs(ctx, buildID)
			if err != nil {
				http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
				log.ErrorContext(ctx, "Failed to fetch logs", slog.Any("error", err))
				return
			}

			logLines = make([]LogLine, len(logs))
			for i, log := range logs {
				logLines[i] = LogLine{
					Number:         uint(i),
					Text:           log.Text,
					TimeSinceStart: log.Timestamp.Sub(*build.Started),
				}
			}
		}

		params := &BuildDetailsPage{
			ID:        build.ID,
			RepoOwner: build.Repo.Owner,
			RepoName:  build.Repo.Name,
			Status:    buildStatus(*build),
			Message:   shortCommitMessage(build.Message),
			Number:    build.Number,
			Started:   build.Started,
			Duration:  durationSinceBuildStart(*build),
			LogLines:  logLines,
		}

		var b bytes.Buffer
		err = tmpl.ExecuteTemplate(&b, "page_build_details", params)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = b.WriteTo(w)
	}
}

func durationSinceBuildStart(b store.Build) *time.Duration {
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

func buildStatus(b store.Build) string {
	if b.Result != nil {
		return string(*b.Result)
	} else if b.Started != nil {
		return "running"
	}
	return "pending"
}

func shortCommitMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	return strings.SplitN(trimmed, "\n", 2)[0]
}

func handleLogStream(s store.PGStore, l store.LogStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid build ID", http.StatusNotFound)
			return
		}

		build, err := s.GetBuild(ctx, buildID)
		if err != nil {
			http.Error(w, "Failed to fetch build", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to fetch build", slog.Any("error", err))
			return
		}

		if build == nil {
			http.Error(w, "Build not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		rc := http.NewResponseController(w)

		for i := range 10 {
			// Check for client disconnect
			ticker := time.NewTicker(1000 * time.Millisecond)
			defer ticker.Stop()

			select {
			case <-ctx.Done():
				log.DebugContext(ctx, "Client disconnected")
				return

				// TODO: use time.After
			case <-ticker.C:
				logLine := LogLine{
					Number:         uint(i),
					Text:           fmt.Sprintf("This is SSE line %d", i),
					TimeSinceStart: time.Second,
				}
				b := bytes.Buffer{}
				err := tmpl.ExecuteTemplate(&b, "comp_log_line", logLine)
				if err != nil {
					log.ErrorContext(ctx, "Failed to write logs", slog.Any("error", err))
					return
				}

				fmt.Println(b.String())
				lines := strings.Split(strings.TrimSpace(b.String()), "\n")
				fmt.Fprintf(w, "event: log-line\ndata: %s\n\n", strings.Join(lines, "\ndata: "))

				rc.Flush()
			}
		}

		fmt.Fprint(w, "event: end-stream\ndata:\n\n")
	}
}
