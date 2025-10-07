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

type LogStore struct {
	LogDir string
}

type LogEntry struct {
	Stream    LogStream `json:"stream"`
	Timestamp time.Time `json:"timestamp"`
	Text      string    `json:"text"`
}

func (s LogStore) GetLogs(ctx context.Context, buildID uint64) ([]LogEntry, error) {
	logFile := path.Join(s.LogDir, fmt.Sprintf("%d.jsonl", buildID))

	file, err := os.Open(logFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return no logs
		}
		return nil, fmt.Errorf("failed to open log file '%s': %w", logFile, err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var logs []LogEntry

	for {
		var entry LogEntry
		err := decoder.Decode(&entry)

		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, fmt.Errorf("failed to decode log entry from '%s': %w", logFile, err)
		}

		logs = append(logs, entry)
	}

	return logs, nil
}
