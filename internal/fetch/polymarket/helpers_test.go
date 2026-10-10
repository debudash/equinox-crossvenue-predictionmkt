package polymarket

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// require is a small stand-in for testify's require (testify is not in go.mod).
var require checks

type checks struct{}

func msg(args []any) string {
	if len(args) == 0 {
		return ""
	}
	if f, ok := args[0].(string); ok {
		return ": " + fmt.Sprintf(f, args[1:]...)
	}
	return ": " + fmt.Sprint(args...)
}

func (checks) NoError(t *testing.T, err error, args ...any) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error %v%s", err, msg(args))
	}
}

func (checks) Equal(t *testing.T, want, got any, args ...any) {
	t.Helper()
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("not equal%s\nwant: %#v\n got: %#v", msg(args), want, got)
	}
}

func (checks) True(t *testing.T, b bool, args ...any) {
	t.Helper()
	if !b {
		t.Fatalf("expected true%s", msg(args))
	}
}

func (checks) False(t *testing.T, b bool, args ...any) {
	t.Helper()
	if b {
		t.Fatalf("expected false%s", msg(args))
	}
}

func (checks) Len(t *testing.T, x any, n int, args ...any) {
	t.Helper()
	if l := reflect.ValueOf(x).Len(); l != n {
		t.Fatalf("length %d, want %d%s", l, n, msg(args))
	}
}

func (checks) Empty(t *testing.T, x any, args ...any) {
	t.Helper()
	v := reflect.ValueOf(x)
	if x != nil && !(v.Kind() == reflect.String || v.Kind() == reflect.Slice || v.Kind() == reflect.Map) {
		t.Fatalf("Empty on %T", x)
	}
	if x != nil && v.Len() != 0 {
		t.Fatalf("expected empty, got %d elements%s", v.Len(), msg(args))
	}
}

// Contains checks a substring of a string or a key of a map.
func (checks) Contains(t *testing.T, in, item any, args ...any) {
	t.Helper()
	if s, ok := in.(string); ok {
		if !strings.Contains(s, item.(string)) {
			t.Fatalf("%q does not contain %q%s", s, item, msg(args))
		}
		return
	}
	if !reflect.ValueOf(in).MapIndex(reflect.ValueOf(item)).IsValid() {
		t.Fatalf("map has no key %v%s", item, msg(args))
	}
}
