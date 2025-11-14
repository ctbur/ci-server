package ui

import (
	"fmt"
	"net/http"
	"strings"
)

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
	for line := range strings.SplitSeq(data, "\n") {
		fmt.Fprintf(w.w, "data: %s\n", line)
	}
	// Two newlines to separate events
	fmt.Fprintln(w.w)

	return w.rc.Flush()
}
