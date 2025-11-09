package store

import (
	"bufio"
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

	// sec: Path is from a trusted user
	file, err := os.Open(logFile) // #nosec G304
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

func (s LogStore) TailLogs(buildID uint64, fromLine uint) *LogTailer {
	return &LogTailer{
		logFilePath: path.Join(s.LogDir, fmt.Sprintf("%d.jsonl", buildID)),
		fromLine:    fromLine,
	}
}

type LogTailer struct {
	logFilePath string
	logFile     *os.File
	logReader   *bufio.Reader
	fromLine    uint
	currentLine uint
}

func (t *LogTailer) Read() ([]LogEntry, error) {
	// Open log file if not yet open
	if t.logReader == nil {
		file, err := os.Open(t.logFilePath)
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		} else if err != nil {
			return nil, err
		}

		t.logFile = file
		t.logReader = bufio.NewReader(file)
	}

	// Read until blocked
	var logEntries []LogEntry
	for {
		line, err := t.logReader.ReadString('\n')
		if errors.Is(err, io.EOF) {
			return logEntries, nil
		} else if err != nil {
			return logEntries, err
		}

		var logEntry LogEntry
		err = json.Unmarshal([]byte(line), &logEntry)
		logEntries = append(logEntries, logEntry)
	}
}

func (t *LogTailer) Close() error {
	if t.logFile != nil {
		return t.logFile.Close()
	}
	return nil
}
