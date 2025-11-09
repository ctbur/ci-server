package ui

import (
	"strings"
	"time"

	"github.com/ctbur/ci-server/v2/internal/store"
)

func durationSinceBuildStart(b store.Build) *time.Duration {
	if b.Started == nil {
		return nil
	}

	var duration time.Duration
	if b.Finished != nil {
		duration = b.Finished.Sub(*b.Started)
	} else {
		duration = time.Since(*b.Started)
	}
	return &duration
}

func buildStatus(b store.Build) string {
	if b.Result != nil {
		return string(*b.Result)
	} else if b.Started != nil {
		return "running"
	}
	return "pending"
}

func shortCommitMessage(msg string) string {
	trimmed := strings.TrimSpace(msg)
	return strings.SplitN(trimmed, "\n", 2)[0]
}
