package build

import (
	"os"
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
)

func TestRunInBuildContext(t *testing.T) {
	logFile, err := os.CreateTemp("", "*.jsonl")
	assert.NoError(t, err, "Failed to create log file")
	defer func() {
		logFile.Close()
		_ = os.Remove(logFile.Name())
	}()

	buildDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err, "Failed to create build dir")
	defer func() {
		_ = os.RemoveAll(buildDir)
	}()

	exitCode, err := runInBuildContext(
		buildDir,
		[]string{"echo", "test"},
		nil,
		make(map[string]string),
		logFile,
	)

	assert.NoError(t, err, "Error when running cmd")
	assert.Equal(t, exitCode, 0, "Unexpected exit code")
}
