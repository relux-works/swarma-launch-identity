package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

const frame = "syn1:dabc9bcf3b62c00fb8cefde216710dbae06b8e96bef04b8cdde47cf3f4e746f3\n"

func args() []string {
	return []string{"derive", "--host-id", "h", "--project-id", "p", "--profile-id", "r"}
}
func absent(string) (string, bool) { return "", false }
func check(t *testing.T, argv []string, lookup Lookup, code string, exit int) {
	t.Helper()
	var out, diag bytes.Buffer
	got := Run(argv, lookup, &out, &diag)
	if got != exit {
		t.Fatalf("exit: got %d want %d", got, exit)
	}
	if exit == 0 {
		if out.String() != frame || len(out.Bytes()) != 70 || diag.Len() != 0 {
			t.Fatal("exact success frame mismatch")
		}
	} else if out.Len() != 0 || diag.String() != "ERR "+code+"\n" {
		t.Fatalf("refusal frame mismatch: stdout length %d diagnostic matches %t", out.Len(), diag.String() == "ERR "+code+"\n")
	}
}

// Shape precedes ID validation and all environment reads. Empty supplied IDs
// are E_ID, while incomplete shapes remain E_USAGE even with an empty ID.
func TestUsageAndPrecedence(t *testing.T) {
	check(t, args(), absent, "", 0)
	never := func(string) (string, bool) { t.Fatal("lookup before valid shape/IDs"); return "", false }
	bad := [][]string{nil, {"other"}, {"derive"}, args()[:6], append(args(), "extra"), {"derive", "--host-id", "h", "--host-id", "p", "--profile-id", "r"}, {"derive", "--unknown", "h", "--project-id", "p", "--profile-id", "r"}, {"derive", "h", "h", "--project-id", "p", "--profile-id", "r"}, {"derive", "--host-id=", "h", "--project-id", "p", "--profile-id", "r"}, {"derive", "--host-id", "", "--project-id", "p", "--profile-id"}}
	for i, a := range bad {
		t.Run(string(rune('a'+i)), func(t *testing.T) { check(t, a, never, "E_USAGE", 2) })
	}
	a := args()
	a[2] = ""
	check(t, a, never, "E_ID", 1)
	a = []string{"derive", "--profile-id", "r", "--host-id", "h", "--project-id", "p"}
	check(t, a, absent, "", 0)
}

// Every representation refusal has a neighboring success. Lookup is forbidden
// until all fields pass, including when an explicit identity would also refuse.
func TestIDsAndBounds(t *testing.T) {
	for _, i := range []int{2, 4, 6} {
		for _, v := range []string{"", "\xff", "\x00", "\u0085", strings.Repeat("x", 257), strings.Repeat("é", 128) + "x"} {
			check(t, args(), absent, "", 0)
			a := args()
			a[i] = v
			check(t, a, func(string) (string, bool) { t.Fatal("lookup on invalid ID"); return "explicit", true }, "E_ID", 1)
		}
		for _, v := range []string{strings.Repeat("x", 256), strings.Repeat("é", 128), "--opaque", "\u200d"} {
			a := args()
			a[i] = v
			var out, diag bytes.Buffer
			if Run(a, absent, &out, &diag) != 0 || out.Len() != 70 || diag.Len() != 0 {
				t.Fatal("valid byte bound/opaque ID refused")
			}
		}
	}
}

// Explicit presence (even empty) precedes transport conflict; existing equal
// transport is admitted. The lookup seam is read-only and checks exact keys.
func TestEnvironmentOrder(t *testing.T) {
	cases := []struct {
		explicit     string
		hasExplicit  bool
		transport    string
		hasTransport bool
		code         string
	}{
		{"", false, "", false, ""}, {"", false, strings.TrimSuffix(frame, "\n"), true, ""},
		{"", true, "", true, "E_EXPLICIT_PRESENT"}, {"named", true, "different", true, "E_EXPLICIT_PRESENT"},
		{"", false, "", true, "E_IDENTITY_CONFLICT"}, {"", false, "different", true, "E_IDENTITY_CONFLICT"},
	}
	for _, c := range cases {
		calls := []string{}
		env := map[string]string{}
		if c.hasExplicit {
			env[ExplicitKey] = c.explicit
		}
		if c.hasTransport {
			env[TransportKey] = c.transport
		}
		before := len(env)
		lookup := func(k string) (string, bool) { calls = append(calls, k); v, ok := env[k]; return v, ok }
		exit := 0
		if c.code != "" {
			exit = 1
		}
		check(t, args(), lookup, c.code, exit)
		want := []string{ExplicitKey}
		if !c.hasExplicit {
			want = append(want, TransportKey)
		}
		if strings.Join(calls, ",") != strings.Join(want, ",") || len(env) != before || env[ExplicitKey] != c.explicit || env[TransportKey] != c.transport {
			t.Fatal("lookup order or state preservation violated")
		}
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// All output is a single bounded 70-byte attempt. Short, failing and panicking
// sinks refuse E_OUTPUT; a partial frame may remain and must be discarded.
func TestOutput(t *testing.T) {
	check(t, args(), absent, "", 0)
	for _, mode := range []string{"success", "short", "failure", "full-error", "panic"} {
		calls := 0
		var out, diag bytes.Buffer
		w := writerFunc(func(p []byte) (int, error) {
			calls++
			if !bytes.Equal(p, []byte(frame)) {
				t.Fatal("bounded exact output attempt mismatch")
			}
			switch mode {
			case "short":
				out.Write(p[:9])
				return 9, nil
			case "failure":
				return 0, errors.New("private writer detail")
			case "full-error":
				out.Write(p)
				return len(p), errors.New("private writer detail")
			case "panic":
				out.Write(p[:9])
				panic("private panic detail")
			}
			return out.Write(p)
		})
		exit := Run(args(), absent, w, &diag)
		if calls != 1 {
			t.Fatal("output write retried")
		}
		if mode == "success" {
			if exit != 0 || out.String() != frame || diag.Len() != 0 {
				t.Fatal("success mismatch")
			}
		} else if exit != 1 || diag.String() != "ERR E_OUTPUT\n" {
			t.Fatal("output refusal mismatch")
		}
	}
}

// Unexpected pre-output failures map to E_INTERNAL, without panic/input data.
// The neighboring valid lookup passes; diagnostic-sink failure stays contained.
func TestInternal(t *testing.T) {
	check(t, args(), absent, "", 0)
	for _, at := range []string{ExplicitKey, TransportKey} {
		check(t, args(), func(k string) (string, bool) {
			if k == at {
				panic("private sentinel")
			}
			return "", false
		}, "E_INTERNAL", 1)
	}
	var out bytes.Buffer
	if Run(nil, absent, &out, writerFunc(func([]byte) (int, error) { panic("private detail") })) != 2 || out.Len() != 0 {
		t.Fatal("broken diagnostic sink escaped")
	}
}

// The checker rejects a deliberately planted value leak; evidence records
// only a boolean verdict. It also rejects extra bytes and success on refusal.
func TestDiagnosticCheckerPlant(t *testing.T) {
	sentinel := "fixture-sensitive-value"
	valid := func(exit int, out, diag string) bool {
		return exit == 1 && out == "" && diag == "ERR E_ID\n" && !strings.Contains(diag, sentinel)
	}
	if !valid(1, "", "ERR E_ID\n") || valid(1, "", "ERR E_ID "+sentinel+"\n") || valid(1, "", "ERR E_ID\n\n") || valid(0, "", "ERR E_ID\n") {
		t.Fatal("diagnostic checker failed known violation")
	}
	var out, diag bytes.Buffer
	a := args()
	a[2] = sentinel + "\x00"
	if !valid(Run(a, absent, &out, &diag), out.String(), diag.String()) {
		t.Fatal("diagnostic privacy contract violated")
	}
}
