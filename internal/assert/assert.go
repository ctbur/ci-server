package assert

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func NoError(t *testing.T, err error, msg string) {
	t.Helper()

	if err != nil {
		t.Errorf("%s: %v", msg, err)
	}
}
func ErrorIs(t *testing.T, err, target error, msg string) {
	t.Helper()

	if !errors.Is(err, target) {
		t.Errorf("%s: got error %v, want %v", msg, err, target)
	}
}

func Equal[V comparable](t *testing.T, got V, want V, msg string) {
	t.Helper()

	if got != want {
		t.Errorf("%s: got %v, want %v", msg, got, want)
	}
}

func DeepEqual[V any](t *testing.T, got V, want V, msg string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n\tGot:  %#v\n\tWant: %#v", msg, got, want)
	}
}

func ElementsMatch[V comparable](t *testing.T, got []V, want []V, msg string) {
	t.Helper()

	var missing []V
	for _, e := range want {
		if !slices.Contains(got, e) {
			missing = append(missing, e)
		}
	}

	var unexpected []V
	for _, e := range got {
		if !slices.Contains(want, e) {
			unexpected = append(unexpected, e)
		}
	}

	if len(missing) == 0 && len(unexpected) == 0 {
		return
	}

	var errMsg strings.Builder
	if len(missing) > 0 {
		errMsg.WriteString("\tMissing elements:\n")
		for _, e := range missing {
			errMsg.WriteString(fmt.Sprintf("\t\t- %#v\n", e))
		}
		errMsg.WriteString("\n")
	}
	if len(unexpected) > 0 {
		errMsg.WriteString("\tUnexpected elements:\n")
		for _, e := range unexpected {
			errMsg.WriteString(fmt.Sprintf("\t\t- %#v\n", e))
		}
		errMsg.WriteString("\n")
	}

	t.Errorf("%s:\n%s", msg, errMsg.String())
}
