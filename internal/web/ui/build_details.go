package ui

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type LogLine struct {
	Number         uint
	Text           string
	TimeSinceStart time.Duration
}

type BuildDetailsPage struct {
	ID            uint64
	RepoOwner     string
	RepoName      string
	Status        string
	Message       string
	Number        uint64
	Started       *time.Time
	Duration      *time.Duration
	LogLines      []LogLine
	LastLogLineNr int
}

func HandleBuildDetails(db *store.DBStore, fs *store.FSStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		params, ok := getBuildDetailsParams(db, fs, w, r)
		if !ok {
			return
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

func HandleBuildDetailsFragment(db *store.DBStore, fs *store.FSStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		params, ok := getBuildDetailsParams(db, fs, w, r)
		if !ok {
			return
		}

		var b bytes.Buffer
		err := tmpl.ExecuteTemplate(&b, "resp_build_details_update", params)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = b.WriteTo(w)
	}
}

func getBuildDetailsParams(db *store.DBStore, fs *store.FSStore, w http.ResponseWriter, r *http.Request) (*BuildDetailsPage, bool) {
	ctx := r.Context()
	log := ctxlog.FromContext(ctx)

	buildID, err := strconv.ParseUint(r.PathValue("build_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid build ID", http.StatusNotFound)
		return nil, false
	}

	fromLine := 0
	if fromLineStr := r.URL.Query().Get("fromLine"); fromLineStr != "" {
		id, err := strconv.ParseInt(fromLineStr, 10, 32)
		if err != nil || id < 0 {
			http.Error(w, "Invalid fromLine parameter", http.StatusBadRequest)
			return nil, false
		}
		fromLine = int(id)
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

	var logLines []LogLine
	if build.Started != nil {
		logs, err := fs.GetLogs(ctx, build.ID, fromLine)
		if err != nil {
			http.Error(w, "Failed to fetch logs", http.StatusInternalServerError)
			log.ErrorContext(ctx, "Failed to fetch logs", slog.Any("error", err))
			return nil, false
		}

		logLines = make([]LogLine, len(logs))
		for i, log := range logs {
			logLines[i] = LogLine{
				// sec: Overflow not practical
				Number:         uint(fromLine + i), // #nosec G115
				Text:           log.Text,
				TimeSinceStart: log.Timestamp.Sub(*build.Started),
			}
		}
	}

	return &BuildDetailsPage{
		ID:            build.ID,
		RepoOwner:     build.Repo.Owner,
		RepoName:      build.Repo.Name,
		Status:        buildStatus(*build),
		Message:       shortCommitMessage(build.Message),
		Number:        build.Number,
		Started:       build.Started,
		Duration:      durationSinceBuildStart(*build),
		LogLines:      logLines,
		LastLogLineNr: fromLine + len(logLines),
	}, true
}
