package ui

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/ctbur/ci-server/v2/internal/ctxlog"
	"github.com/ctbur/ci-server/v2/internal/store"
)

func HandleLogin(db *store.DBStore, tmpl *template.Template) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := ctxlog.FromContext(ctx)

		var b bytes.Buffer
		err := tmpl.ExecuteTemplate(&b, "page_login", nil)
		if err != nil {
			http.Error(w, "Failed to render template", http.StatusInternalServerError)
			log.Error("Failed to render template", slog.Any("error", err))
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = b.WriteTo(w)
	}
}
