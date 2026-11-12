package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The helper runs the real main in a disposable subprocess, with no launcher
// or credentials. Only test code starts children; the utility never does.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("BRIDGE_TEST_HELPER") != "1" {
		return
	}
	for i, a := range os.Args {
		if a == "--" {
			os.Args = append([]string{"swarma-launch-identity"}, os.Args[i+1:]...)
			main()
			return
		}
	}
	os.Exit(99)
}

// Real process frames and exit codes remain exact across directory, process
// and fake profile-version changes. This does not qualify a receiving route.
func TestProcess(t *testing.T) {
	const want = "syn1:dabc9bcf3b62c00fb8cefde216710dbae06b8e96bef04b8cdde47cf3f4e746f3\n"
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	valid := []string{"derive", "--host-id", "h", "--project-id", "p", "--profile-id", "r"}
	cases := []struct {
		extra []string
		argv  []string
		exit  int
		diag  string
	}{
		{nil, valid, 0, ""}, {[]string{"PROFILE_VERSION=next", "HOSTNAME=unrelated", "PWD=unrelated", "PID=999"}, valid, 0, ""},
		{[]string{"A2A_AGENT="}, valid, 1, "ERR E_EXPLICIT_PRESENT\n"},
		{[]string{"SWARMA_LAUNCH_IDENTITY="}, valid, 1, "ERR E_IDENTITY_CONFLICT\n"},
		{[]string{"SWARMA_LAUNCH_IDENTITY=" + strings.TrimSuffix(want, "\n")}, valid, 0, ""},
		{nil, []string{"derive", "--host-id", "", "--project-id", "p", "--profile-id", "r"}, 1, "ERR E_ID\n"},
		{nil, []string{"derive"}, 2, "ERR E_USAGE\n"},
	}
	for _, c := range cases {
		dir, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		argv := append([]string{"-test.run=^TestHelperProcess$", "--"}, c.argv...)
		cmd := exec.Command(binary, argv...)
		cmd.Dir = dir
		cmd.Env = append([]string{"BRIDGE_TEST_HELPER=1"}, c.extra...)
		var out, diag bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &diag
		err = cmd.Run()
		exit := 0
		if err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				exit = e.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		expected := ""
		if c.exit == 0 {
			expected = want
		}
		if exit != c.exit || out.String() != expected || diag.String() != c.diag {
			t.Fatalf("process frame/exit mismatch: exit %d stdout length %d diagnostic matches %t", exit, out.Len(), diag.String() == c.diag)
		}
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			t.Fatal("utility changed disposable working directory")
		}
	}
}

// A real closed pipe must return E_OUTPUT/1, not SIGPIPE termination. The
// normal process case above is the neighboring usable-output control.
func TestClosedOutputPipe(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	cmd := exec.Command(binary, "-test.run=^TestHelperProcess$", "--", "derive", "--host-id", "h", "--project-id", "p", "--profile-id", "r")
	cmd.Env = []string{"BRIDGE_TEST_HELPER=1"}
	cmd.Stdout = writer
	var diag bytes.Buffer
	cmd.Stderr = &diag
	err = cmd.Run()
	e, ok := err.(*exec.ExitError)
	if !ok || e.ExitCode() != 1 || diag.String() != "ERR E_OUTPUT\n" {
		t.Fatal("closed stdout must refuse E_OUTPUT/1")
	}
}
