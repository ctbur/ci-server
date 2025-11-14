package ui

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/store"
)

type BuildListPage struct {
	BuildCards   []BuildCard
	PreviousPage *uint
	CurrentPage  uint
	NextPage     *uint
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
		log := ctxlog.FromContext(ctx)

		page, buildCards, ok := getBuildCards(db, w, r)
		if !ok {
			return
		}

		var previousPage *uint
		if page > 0 {
			p := page - 1
			previousPage = &p
		}

		totalBuilds, err := db.CountBuilds(ctx)
		if err != nil {
			http.Error(w, "Failed to count builds", http.StatusInternalServerError)
			log.Error("Failed to count builds", slog.Any("error", err))
			return
		}

		var nextPage *uint
		if uint((page+1)*buildListPageSize) < uint(totalBuilds) {
			n := page + 1
			nextPage = &n
		}

		params := &BuildListPage{
			BuildCards:   buildCards,
			PreviousPage: previousPage,
			CurrentPage:  page,
			NextPage:     nextPage,
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

func HandleBuildListFragment(db *store.DBStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		_, buildCards, ok := getBuildCards(db, w, r)
		if !ok {
			return
		}

		var b bytes.Buffer
		err := tmpl.ExecuteTemplate(&b, "comp_build_list", buildCards)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			log.Error("Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = b.WriteTo(w)
	}
}

var buildListPageSize = uint(12)

func getBuildCards(
	db *store.DBStore, w http.ResponseWriter, r *http.Request,
) (uint, []BuildCard, bool) {
	ctx := r.Context()
	log := ctxlog.FromContext(ctx)

	page := uint(0)
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		id, err := strconv.ParseUint(pageStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid beforeID parameter", http.StatusBadRequest)
			return 0, nil, false
		}
		page = uint(id)
	}

	builds, err := db.ListBuilds(ctx, page, buildListPageSize)
	if err != nil {
		http.Error(w, "Failed to list builds", http.StatusInternalServerError)
		log.Error("Failed to list builds", slog.Any("error", err))
		return 0, nil, false
	}

	buildCards := make([]BuildCard, len(builds))
	for i, b := range builds {
		card := BuildCard{
			ID:        b.ID,
			Status:    buildStatus(b),
			Message:   shortCommitMessage(b.Message),
			Author:    b.Author,
			Ref:       strings.TrimPrefix(b.Ref, "refs/heads/"),
			CommitSHA: b.CommitSHA[:min(7, len(b.CommitSHA))],
			Duration:  durationSinceBuildStart(b),
			Started:   b.Started,
		}
		buildCards[i] = card
	}

	return page, buildCards, true
}
