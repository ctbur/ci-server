package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"
)

type LogStream string

const (
	LogStreamStdout LogStream = "out"
	LogStreamStderr LogStream = "err"
)

type LogEntry struct {
	Stream    LogStream `json:"stream"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
}

func (fs *FSStore) GetLogs(ctx context.Context, buildID uint64, fromLine int) ([]LogEntry, error) {
	LogFilePath := path.Join(fs.RootDir, "build-logs", fmt.Sprintf("%d.jsonl", buildID))

	// sec: Path is from a trusted user
	logFile, err := os.Open(LogFilePath) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return no logs
		}
		return nil, fmt.Errorf("failed to open log file '%s': %w", LogFilePath, err)
	}
	defer logFile.Close()

	decoder := json.NewDecoder(logFile)
	var logs []LogEntry

	currentLine := 0
	for {
		var entry LogEntry
		err := decoder.Decode(&entry)

		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to decode log entry from '%s': %w", LogFilePath, err)
		}

		// Skip lines until we reach fromLine - inefficient but good enough for now
		if currentLine >= fromLine {
			logs = append(logs, entry)
		}
		currentLine++
	}

	return logs, nil
}
