package ui

import (
	"bytes"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
	"github.com/ctbur/ci-server/v2/internal/web/wlog"
)

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

func HandleBuildDetails(db *store.DBStore, l store.LogStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		build, ok := getBuildFromPath(db, w, r)
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
					// sec: Overflow not practical
					Number:         uint(i), // #nosec G115
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

const LogPollPeriod = 500 * time.Millisecond

func HandleLogStream(db *store.DBStore, l store.LogStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := wlog.FromContext(ctx)

		build, ok := getBuildFromPath(db, w, r)
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
			build, err := db.GetBuild(ctx, build.ID)
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

		if err := sseWriter.sendEvent("", "build-started", ""); err != nil {
			log.ErrorContext(ctx, "Failed to send event", slog.Any("error", err))
			return
		}

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

				if err := sseWriter.sendEvent(strconv.Itoa(i), "log-line", b.String()); err != nil {
					log.ErrorContext(ctx, "Failed to send event", slog.Any("error", err))
					return
				}
				logNr++
			}

			build, err := db.GetBuild(ctx, build.ID)
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

		if err := sseWriter.sendEvent("", "build-finished", ""); err != nil {
			log.ErrorContext(ctx, "Failed to send event", slog.Any("error", err))
			return
		}
	}
}

func getBuildFromPath(db *store.DBStore, w http.ResponseWriter, r *http.Request) (*store.Build, bool) {
	ctx := r.Context()
	log := wlog.FromContext(ctx)

	buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid build ID", http.StatusNotFound)
		return nil, false
	}

	build, err := db.GetBuild(ctx, buildID)
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
