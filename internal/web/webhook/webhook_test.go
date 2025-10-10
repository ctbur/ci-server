package webhook

import (
	"strings"
	"testing"

	"github.com/ctbur/ci-server/v2/internal/assert"
	"github.com/ctbur/ci-server/v2/internal/store"
)

func longString(length int) string {
	return strings.Repeat("A", length)
}

func TestSanitizeBuildMeta(t *testing.T) {
	// A valid baseline store.BuildMeta struct
	validMeta := store.BuildMeta{
		Link:      "https://github.com/test/repo/commit/0123456789abcdef0123456789abcdef01234567",
		Ref:       "refs/heads/main",
		CommitSHA: "0123456789abcdef0123456789abcdef01234567", // 40 chars
		Message:   "Test commit message",
		Author:    "tester-mctestface",
	}

	tests := []struct {
		name           string
		input          store.BuildMeta
		expectedErrors []string // Simple string messages
	}{
		// --- Success Case ---
		{
			name:           "Success_ValidInput",
			input:          validMeta,
			expectedErrors: nil,
		},

		// --- Ref Rejection Cases ---
		{
			name:           "Ref_Required",
			input:          store.BuildMeta{Ref: "", CommitSHA: validMeta.CommitSHA, Author: validMeta.Author},
			expectedErrors: []string{"Ref is required", "Ref must start with 'refs/'"},
		},
		{
			name: "Ref_MissingPrefix",
			input: store.BuildMeta{
				Ref:       "heads/main",
				CommitSHA: validMeta.CommitSHA,
				Author:    validMeta.Author,
				Message:   validMeta.Message,
			},
			expectedErrors: []string{"Ref must start with 'refs/'"},
		},
		{
			name:           "Ref_TooLong",
			input:          store.BuildMeta{Ref: longString(256), CommitSHA: validMeta.CommitSHA, Author: validMeta.Author, Message: validMeta.Message},
			expectedErrors: []string{"Ref must start with 'refs/'", "Ref must be fewer than 255 characters"},
		},

		// --- Commit SHA Rejection Cases ---
		{
			name:           "SHA_Required_And_WrongLength",
			input:          store.BuildMeta{Ref: validMeta.Ref, Author: validMeta.Author, Message: validMeta.Message},
			expectedErrors: []string{"Commit SHA is required", "Commit SHA must be 40 characters long"},
		},
		{
			name:           "SHA_InvalidHex_And_WrongLength",
			input:          store.BuildMeta{CommitSHA: "0123456789abcdef0123456789abcdef0123456Z", Ref: validMeta.Ref, Author: validMeta.Author, Message: validMeta.Message},
			expectedErrors: []string{"Commit SHA must be hex"},
		},
		{
			name:           "SHA_WrongLength",
			input:          store.BuildMeta{CommitSHA: "0123456789abcdef0123456789abcdef0123456", Ref: validMeta.Ref, Author: validMeta.Author, Message: validMeta.Message}, // 39 chars
			expectedErrors: []string{"Commit SHA must be 40 characters long"},
		},

		// --- Author Rejection Cases ---
		{
			name:           "Author_Required",
			input:          store.BuildMeta{Ref: validMeta.Ref, CommitSHA: validMeta.CommitSHA, Message: validMeta.Message},
			expectedErrors: []string{"Author is required"},
		},
		{
			name:           "Author_TooLong",
			input:          store.BuildMeta{Author: longString(101), Ref: validMeta.Ref, CommitSHA: validMeta.CommitSHA, Message: validMeta.Message},
			expectedErrors: []string{"Author must be fewer than 100 characters"},
		},

		// --- Link Rejection Cases ---
		{
			name:           "Link_InvalidPrefix",
			input:          store.BuildMeta{Link: "http://bad.link", Ref: validMeta.Ref, CommitSHA: validMeta.CommitSHA, Author: validMeta.Author, Message: validMeta.Message},
			expectedErrors: []string{"Link must start with 'https://'"},
		},
		{
			name:           "Link_TooLong",
			input:          store.BuildMeta{Link: "https://" + longString(248), Ref: validMeta.Ref, CommitSHA: validMeta.CommitSHA, Author: validMeta.Author, Message: validMeta.Message}, // 256 chars total
			expectedErrors: []string{"Link must be fewer than 256 characters"},
		},

		// --- Multiple Errors Case ---
		{
			name: "MultipleErrors_LinkRefSHA",
			input: store.BuildMeta{
				Link:      "http://bad.link",
				Ref:       "heads/main",
				CommitSHA: "0123456789abcdef0123456789abcde",
				Message:   validMeta.Message,
				Author:    validMeta.Author,
			},
			expectedErrors: []string{
				"Ref must start with 'refs/'",
				"Commit SHA must be 40 characters long",
				"Link must start with 'https://'",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh copy of the input struct for testing
			b := tt.input

			err := sanitizeBuild(&b)

			if len(tt.expectedErrors) == 0 {
				// Check for success
				assert.NoError(t, err, "Mistakenly returned an error")
				return
			}

			// Check for failure
			errVal, ok := err.(*ValidationError)
			assert.Equal(t, ok, true, "Error is not of type ValidationError")
			assert.ElementsMatch(t, errVal.Errors, tt.expectedErrors, "Validation errors did not match expected set.")
		})
	}
}

func TestSanitizeBuildMeta_MessageCap(t *testing.T) {
	maxLen := 1000
	b := store.BuildMeta{
		Ref:       "refs/heads/feature",
		CommitSHA: "0123456789ABCDEF0123456789ABCDEF01234567",
		Author:    "Test User",
		Message:   longString(maxLen + 50),
	}

	err := sanitizeBuild(&b)
	assert.NoError(t, err, "sanitizeBuild failed")

	actualLen := len([]rune(b.Message))
	assert.Equal(t, actualLen, maxLen, "Message capping incorrect")

	assert.Equal(t, b.CommitSHA, "0123456789abcdef0123456789abcdef01234567", "Commit SHA not lowercase")
}
