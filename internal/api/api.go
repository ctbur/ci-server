package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/ctbur/ci-server/v2/internal/build"
	"github.com/ctbur/ci-server/v2/internal/disk"
	"github.com/go-chi/chi/v5"
)

type API struct {
}

func New() API {
	return API{}
}

func (a API) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(loggerMiddleware)

	r.Route("/webhook", func(r chi.Router) {
		r.Post("/", handleWebhook())
	})

	return r
}

type loggerKey struct{}

func LoggerFromContext(ctx context.Context) *slog.Logger {
	logger := ctx.Value(loggerKey{})
	return logger.(*slog.Logger)
}

func loggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		log := slog.New(slog.NewTextHandler(os.Stdout, nil))
		log = log.With(
			slog.String("method", r.Method),
			slog.String("request", r.RequestURI),
		)

		ctx = context.WithValue(ctx, loggerKey{}, log)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(ctx))
		end := time.Now()

		duration := end.Sub(start).Milliseconds()

		log.Info(
			"request handled",
			slog.Int64("latency", duration),
			slog.String("time", end.Format(time.RFC3339)),
		)
	})
}

func handleWebhook() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log := LoggerFromContext(r.Context())
		log.Info("HandleWebhook called")

		dd := disk.New("/data")
		builder := build.NewBuilder(&dd)

		buildCmd := build.BuildCmd{
			BuildImage: "",
			Cmd:        []string{},
		}

		commitSHA := "4186f57fe872a9a8aa6ffd9e7857588a9c6eada2"
		err := builder.Build("godotengine", "godot", "https://github.com/godotengine/godot.git", commitSHA, buildCmd)

		if err != nil {
			renderError(w, err, 500)
		} else {
			renderStruct(w, struct{}{}, 200)
		}
	}
}

func renderStruct(w http.ResponseWriter, v interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.Encode(v)
}

func renderError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)

	errStr := fmt.Sprintf("%v", err)
	enc.Encode(struct {
		Error string `json:"error"`
	}{Error: errStr})
}
