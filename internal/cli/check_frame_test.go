package cli

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

const checkFrameName = "syn1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func checkFrameArgs(name string) []string {
	return []string{"check-frame", "--expected", name}
}

func assertCheckFrameSuccess(t *testing.T, name string, input []byte) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if got := CheckFrame(checkFrameArgs(name), bytes.NewReader(input), &stdout, &stderr); got != 0 {
		t.Fatalf("success exit: got %d", got)
	}
	if stdout.Len() != 69 || stdout.String() != name || stderr.Len() != 0 {
		t.Fatal("success did not write exactly the expected 69 bytes with empty stderr")
	}
}

func assertCheckFrameRefusal(t *testing.T, args []string, input io.Reader, code string, status int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if got := CheckFrame(args, input, &stdout, &stderr); got != status {
		t.Fatalf("refusal exit: got %d want %d", got, status)
	}
	if stdout.Len() != 0 || stderr.String() != "ERR "+code+"\n" {
		t.Fatalf("refusal output mismatch for %s", code)
	}
}

// The parser accepts only the exact command shape; every malformed form has a
// nearby valid invocation and refuses before reading stdin.
func TestCheckFrameUsage(t *testing.T) {
	for i, bad := range [][]string{
		{}, {"derive"}, {"check-frame"}, {"check-frame", "--expected"},
		{"check-frame", "--unknown", checkFrameName},
		{"check-frame", "--expected=" + checkFrameName},
		{"check-frame", checkFrameName},
		{"check-frame", "--expected", checkFrameName, "extra"},
		{"check-frame", "--expected", checkFrameName, "--expected", checkFrameName},
	} {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
			input := &byteCountingReader{reader: bytes.NewReader([]byte(checkFrameName + "\n"))}
			assertCheckFrameRefusal(t, bad, input, "E_USAGE", 2)
			if input.consumed != 0 {
				t.Fatal("usage refusal consumed stdin")
			}
		})
	}
}

// Expected names use the exact lowercase syn1 form; invalid spellings refuse
// E_ID before input is read and never appear in diagnostics.
func TestCheckFrameExpectedName(t *testing.T) {
	badNames := []string{
		"", "fixture-private-name", "syn1:", "syn1:" + strings.Repeat("a", 63),
		"syn1:" + strings.Repeat("A", 64), "syn2:" + strings.Repeat("a", 64),
		"syn1:" + strings.Repeat("a", 65), "syn1:" + strings.Repeat("é", 32),
	}
	for i, name := range badNames {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
			input := &byteCountingReader{reader: bytes.NewReader([]byte(checkFrameName + "\n"))}
			assertCheckFrameRefusal(t, checkFrameArgs(name), input, "E_ID", 1)
			if input.consumed != 0 {
				t.Fatal("malformed expected name consumed stdin")
			}
		})
	}
}

// Every invalid raw frame is paired with the exact valid 70-byte control.
func TestCheckFrameFrames(t *testing.T) {
	other := "syn1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cases := []struct {
		name  string
		input []byte
	}{
		{"empty", nil},
		{"partial", []byte(checkFrameName[:17])},
		{"missing LF", []byte(checkFrameName)},
		{"CRLF", []byte(checkFrameName + "\r\n")},
		{"double LF", []byte(checkFrameName + "\n\n")},
		{"trailing byte", []byte(checkFrameName + "\nX")},
		{"several frames", []byte(checkFrameName + "\n" + checkFrameName + "\n")},
		{"unequal valid name", []byte(other + "\n")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
			assertCheckFrameRefusal(t, checkFrameArgs(checkFrameName), bytes.NewReader(tc.input), "E_FRAME", 1)
		})
	}
}

type byteCountingReader struct {
	reader   io.Reader
	consumed int
}

func (r *byteCountingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.consumed += n
	return n, err
}

type repeatingReader struct{ consumed int }

func (r *repeatingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.consumed += len(p)
	return len(p), nil
}

// LimitReader must stop exactly when byte 71 proves an overlong frame; the
// neighboring valid frame remains accepted.
func TestCheckFrameReadLimit(t *testing.T) {
	assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
	reader := &repeatingReader{}
	assertCheckFrameRefusal(t, checkFrameArgs(checkFrameName), reader, "E_FRAME", 1)
	if reader.consumed != 71 {
		t.Fatalf("reader consumed %d bytes, want 71", reader.consumed)
	}
	failedLongRead := &byteCountingReader{reader: &failingReader{data: []byte(strings.Repeat("x", 100)), err: errors.New("private reader detail")}}
	assertCheckFrameRefusal(t, checkFrameArgs(checkFrameName), failedLongRead, "E_FRAME", 1)
	if failedLongRead.consumed != 71 {
		t.Fatalf("failed long read consumed %d bytes, want 71", failedLongRead.consumed)
	}
}

type failingReader struct {
	data []byte
	err  error
	done bool
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, r.err
	}
	r.done = true
	return copy(p, r.data), r.err
}

// Reader errors are fixed E_INPUT refusals, even after a partial read; a valid
// complete frame is the adjacent positive control.
func TestCheckFrameInputError(t *testing.T) {
	assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
	assertCheckFrameRefusal(t, checkFrameArgs(checkFrameName), &failingReader{data: []byte("partial"), err: errors.New("private reader detail")}, "E_INPUT", 1)
}

type blockedAfterFrameReader struct {
	frame   []byte
	read    bool
	entered chan struct{}
	release chan struct{}
}

func (r *blockedAfterFrameReader) Read(p []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(p, r.frame), nil
	}
	close(r.entered)
	<-r.release
	return 0, io.EOF
}

// An unfinished producer cannot be counted as success: the checker remains
// blocked after 70 bytes until EOF arrives, without using timing sleeps.
func TestCheckFrameWaitsForEOF(t *testing.T) {
	reader := &blockedAfterFrameReader{
		frame: []byte(checkFrameName + "\n"), entered: make(chan struct{}), release: make(chan struct{}),
	}
	done := make(chan struct {
		exit   int
		stdout string
		stderr string
	}, 1)
	go func() {
		var stdout, stderr bytes.Buffer
		exit := CheckFrame(checkFrameArgs(checkFrameName), reader, &stdout, &stderr)
		done <- struct {
			exit   int
			stdout string
			stderr string
		}{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
	}()
	select {
	case <-reader.entered:
	case <-done:
		t.Fatal("checker completed without waiting for producer EOF")
	}
	select {
	case <-done:
		t.Fatal("checker completed before the producer reached EOF")
	default:
	}
	close(reader.release)
	got := <-done
	if got.exit != 0 || got.stdout != checkFrameName || got.stderr != "" {
		t.Fatal("complete frame was not accepted after EOF")
	}
}

// E_OUTPUT reports one bounded 69-byte attempt with a fixed diagnostic. Do
// not require empty stdout: a writer may retain accepted bytes, so the caller
// must discard all captured output on exit 1.
func TestCheckFrameOutput(t *testing.T) {
	type outputCase struct {
		name       string
		accepted   int
		writeError bool
		panicWrite bool
	}
	cases := []outputCase{
		{name: "short write", accepted: 8},
		{name: "failing write", writeError: true},
		{name: "partial write with error", accepted: 8, writeError: true},
		{name: "full write with error", accepted: 69, writeError: true},
		{name: "panic during write", accepted: 8, panicWrite: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
			calls := 0
			var accepted, diagnostic bytes.Buffer
			var attempted []byte
			writer := writerFunc(func(p []byte) (int, error) {
				calls++
				attempted = append([]byte(nil), p...)
				if tc.accepted > 0 {
					_, _ = accepted.Write(p[:tc.accepted])
				}
				if tc.panicWrite {
					panic("private writer panic")
				}
				if tc.writeError {
					return tc.accepted, errors.New("private writer detail")
				}
				return tc.accepted, nil
			})
			exit := CheckFrame(checkFrameArgs(checkFrameName), bytes.NewReader([]byte(checkFrameName+"\n")), writer, &diagnostic)
			if exit != 1 || diagnostic.String() != "ERR E_OUTPUT\n" || calls != 1 ||
				len(attempted) != 69 || string(attempted) != checkFrameName ||
				accepted.Len() != tc.accepted || !bytes.Equal(accepted.Bytes(), []byte(checkFrameName)[:tc.accepted]) {
				t.Fatal("bounded output refusal contract mismatch")
			}
		})
	}
}

type panicReader struct{}

func (panicReader) Read([]byte) (int, error) { panic("private reader panic") }

// Panics before output become E_INTERNAL; the fixed diagnostic hides panic
// details and the neighboring valid frame succeeds.
func TestCheckFrameInternalPanic(t *testing.T) {
	assertCheckFrameSuccess(t, checkFrameName, []byte(checkFrameName+"\n"))
	assertCheckFrameRefusal(t, checkFrameArgs(checkFrameName), panicReader{}, "E_INTERNAL", 1)
	var stdout bytes.Buffer
	if got := CheckFrame(checkFrameArgs(checkFrameName), panicReader{}, &stdout, writerFunc(func([]byte) (int, error) { panic("diagnostic sink") })); got != 1 || stdout.Len() != 0 {
		t.Fatal("panic in input or diagnostic path escaped containment")
	}
}
