package assert

import (
	"errors"
	"reflect"
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
