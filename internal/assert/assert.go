package assert

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type Assert struct {
	t       *testing.T
	failure bool
}

func (a Assert) Fatal() {
	if a.failure {
		a.t.FailNow()
	}
}

func NoError(t *testing.T, err error, msg string) Assert {
	t.Helper()

	if err != nil {
		t.Errorf("%s: %v", msg, err)
		return Assert{t: t, failure: true}
	}
	return Assert{t: t, failure: false}
}

func ErrorIs(t *testing.T, err, target error, msg string) Assert {
	t.Helper()

	if !errors.Is(err, target) {
		t.Errorf("%s: got error %v, want %v", msg, err, target)
		return Assert{t: t, failure: true}
	}
	return Assert{t: t, failure: false}
}

func Equal[V comparable](t *testing.T, got V, want V, msg string) Assert {
	t.Helper()

	if got != want {
		t.Errorf("%s: got %v, want %v", msg, got, want)
		return Assert{t: t, failure: true}
	}
	return Assert{t: t, failure: false}
}

func DeepEqual[V any](t *testing.T, got V, want V, msg string) Assert {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s:\n\tGot:  %#v\n\tWant: %#v", msg, got, want)
		return Assert{t: t, failure: true}
	}
	return Assert{t: t, failure: false}
}

func ElementsMatch[V comparable](t *testing.T, got []V, want []V, msg string) Assert {
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
		return Assert{t: t, failure: false}
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
	return Assert{t: t, failure: true}
}

func FileContents(t *testing.T, name, want, msg string) Assert {
	t.Helper()

	// sec: not required for test code
	data, err := os.ReadFile(name) // #nosec G304
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s: wanted file '%s' does not exist", msg, name)
		} else {
			t.Errorf("%s: error reading file '%s'", msg, name)
		}
		return Assert{t: t, failure: true}
	}

	if string(data) != want {
		t.Errorf("%s: contents of file '%s':\n\tGot: %s\n\tWant: %s", name, msg, data, want)
		return Assert{t: t, failure: true}
	}

	return Assert{t: t, failure: false}
}
