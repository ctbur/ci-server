package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
)

func TestRemoveAll(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "go-remove-test")
	assert.NoError(t, err, "Failed to create temp directory")

	rxDir := filepath.Join(tempDir, "sub-read-only")
	roFile := filepath.Join(rxDir, "read-only-file.txt")

	// Create dir as read/write/execute (rwx)
	if err := os.Mkdir(rxDir, 0700); err != nil {
		t.Fatalf("Failed to create dir: %v", err)
	}

	// Create file as read-only (r--)
	if err := os.WriteFile(roFile, []byte("test content"), 0400); err != nil {
		t.Fatalf("Failed to create read-only file: %v", err)
	}

	// Change dir to read/execute (r-x)
	if err := os.Chmod(rxDir, 0500); err != nil {
		t.Fatalf("Failed to change dir to read-only: %v", err)
	}

	err = removeAll(rxDir)

	if err != nil {
		t.Errorf("removeAll failed: %v", err)
	}

	if _, err := os.Stat(rxDir); !os.IsNotExist(err) {
		t.Errorf("Directory was not deleted. os.Stat returned %v", err)
	}
}
