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

const LogStreamPollInterval = 500 * time.Millisecond

func (s LogStore) StreamLogs(
	ctx context.Context, buildID uint64, fromLine uint64,
) (<-chan LogEntry, <-chan error) {
	logChan := make(chan LogEntry)
	// Error channel is buffered to allow deferred functions to run on failed
	errChan := make(chan error, 1)

	logFile := path.Join(s.LogDir, fmt.Sprintf("%d.jsonl", buildID))

	go func() {
		defer close(logChan)
		defer close(errChan)

		// Wait for file to be created and open it
		var file *os.File
		var openErr error
		for {
			// sec: Path is from a trusted user
			file, openErr = os.Open(logFile) // #nosec G304
			if openErr == nil {
				// Successfully opened file
				break
			}

			if !errors.Is(openErr, os.ErrNotExist) {
				// Other error occurred
				errChan <- fmt.Errorf("failed to open log file '%s': %w", logFile, openErr)
				return
			}

			// File doesn't exist - wait and try again
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case <-time.After(LogStreamPollInterval):
				continue
			}
		}
		defer file.Close()

		// Tail the log file
		reader := bufio.NewReader(file)
		logOffset := uint64(0)
		for {
			// Read lines until blocked
			var line string
			var readErr error
			for {
				line, readErr = reader.ReadString('\n')
				if readErr != nil {
					break
				}

				if logOffset >= fromLine {
					var logEntry LogEntry
					err := json.Unmarshal([]byte(line), &logEntry)
					if err != nil {
						errChan <- fmt.Errorf("failure unmarshalling log line: %w", err)
						return
					}

					// Send log entry
					select {
					// Exit in case log entry is not read before context is cancelled
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					case logChan <- logEntry:
						// Success
					}
				}
				logOffset++
			}

			if !errors.Is(readErr, io.EOF) {
				// Other error occurred
				errChan <- fmt.Errorf("failure while tailing log file '%s': %w", logFile, readErr)
				return
			}

			// EOF was reached before ReadString() found a newline
			// The partial line read is still buffered inside the reader
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			case <-time.After(LogStreamPollInterval):
				continue
			}
		}
	}()

	return logChan, errChan
}
