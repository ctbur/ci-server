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

		build, ok := getBuildFromPath(s, w, r)
		if !ok {
			return
		}

		var logLines []LogLine
		if build.Started != nil {
			logs, err := l.GetLogs(ctx, build.ID)
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
		err := tmpl.ExecuteTemplate(&b, "page_build_details", params)
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

const LogPollPeriod = 500 * time.Millisecond

func handleLogStream(s store.PGStore, l store.LogStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		build, ok := getBuildFromPath(s, w, r)
		if !ok {
			return
		}

		// Resolve starting log line
		fromLineStr := r.URL.Query().Get("fromLine")
		lastEventIDStr := r.Header.Get("Last-Event-Id")

		fromLine := uint(0)
		if lastEventIDStr != "" {
			lastEventID, err := strconv.ParseUint(lastEventIDStr, 10, 32)
			if err != nil {
				http.Error(w, fmt.Sprintf("Last-Event-Id is not a number: %s", lastEventIDStr), http.StatusBadRequest)
				return
			}

			fromLine = uint(lastEventID) + 1
		} else if fromLineStr != "" {
			fromLineU64, err := strconv.ParseUint(fromLineStr, 10, 32)
			if err != nil {
				http.Error(w, fmt.Sprintf("fromLine is not a number: %s", fromLineStr), http.StatusBadRequest)
				return
			}

			fromLine = uint(fromLineU64)
		}

		sseWriter := beginSSE(w)

		// Wait for build to start
		for {
			build, err := s.GetBuild(ctx, build.ID)
			if err != nil {
				log.ErrorContext(ctx, "Failed to get build", slog.Any("error", err))
				return
			}
			if build.Started != nil {
				break
			}

			select {
			case <-ctx.Done():
				return

			case <-time.After(LogPollPeriod):
				continue
			}
		}

		sseWriter.sendEvent("", "build-started", "")

		// Wait for build to end
		logTailer := l.TailLogs(build.ID, fromLine)
		defer logTailer.Close()

		logNr := fromLine
		for {
			logEntries, err := logTailer.Read()
			if err != nil {
				log.ErrorContext(ctx, "Failed to tail logs", slog.Any("error", err))
				return
			}

			for i, logEntry := range logEntries {
				b := bytes.Buffer{}
				err := tmpl.ExecuteTemplate(&b, "comp_log_line", LogLine{
					Number:         logNr,
					Text:           logEntry.Text,
					TimeSinceStart: logEntry.Timestamp.Sub(*build.Started),
				})
				if err != nil {
					log.ErrorContext(ctx, "Failed to run log template", slog.Any("error", err))
					return
				}

				sseWriter.sendEvent(strconv.Itoa(i), "log-line", b.String())
				logNr++
			}

			build, err := s.GetBuild(ctx, build.ID)
			if err != nil {
				log.ErrorContext(ctx, "Failed to get build", slog.Any("error", err))
				return
			}
			if build.Finished != nil {
				break
			}

			select {
			case <-ctx.Done():
				return

			case <-time.After(LogPollPeriod):
				continue
			}
		}

		sseWriter.sendEvent("", "build-finished", "")
	}
}

type SSEWriter struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func beginSSE(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Flush headers immediately
	w.WriteHeader(http.StatusOK)

	rc := http.NewResponseController(w)
	return &SSEWriter{w, rc}
}

func (w *SSEWriter) sendEvent(id, event, data string) error {
	if id != "" {
		fmt.Fprintf(w.w, "id: %s\n", id)
	}

	fmt.Fprintf(w.w, "event: %s\n", event)

	data = strings.TrimSpace(data)
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(w.w, "data: %s\n", line)
	}
	// Two newlines to separate events
	fmt.Fprintln(w.w)

	return w.rc.Flush()
}

func getBuildFromPath(s store.PGStore, w http.ResponseWriter, r *http.Request) (*store.Build, bool) {
	ctx := r.Context()
	log := wlog.FromContext(ctx)

	buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid build ID", http.StatusNotFound)
		return nil, false
	}

	build, err := s.GetBuild(ctx, buildID)
	if err != nil {
		http.Error(w, "Failed to fetch build", http.StatusInternalServerError)
		log.ErrorContext(ctx, "Failed to fetch build", slog.Any("error", err))
		return nil, false
	}

	if build == nil {
		http.Error(w, "Build not found", http.StatusNotFound)
		return nil, false
	}

	return build, true
}
