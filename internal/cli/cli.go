// Package cli implements the closed command interface with explicit I/O seams.
package cli

import (
	"io"

	"github.com/relux-works/swarma-launch-identity"
)

const ExplicitKey = "A2A_AGENT"
const TransportKey = "SWARMA_LAUNCH_IDENTITY"

// Lookup is called only for the two public identity keys, in contract order.
type Lookup func(string) (string, bool)

// Run returns the process exit code. It never writes files, mutates environment,
// starts children or inspects ambient data beyond Lookup's two named fields.
func Run(args []string, lookup Lookup, stdout, stderr io.Writer) (exit int) {
	outputStarted := false
	defer func() {
		if recover() != nil {
			code := "E_INTERNAL"
			if outputStarted {
				code = "E_OUTPUT"
			}
			exit = refuse(stderr, code, 1)
		}
	}()
	ids, ok := parse(args)
	if !ok {
		return refuse(stderr, "E_USAGE", 2)
	}
	if ids.Validate() != nil {
		return refuse(stderr, "E_ID", 1)
	}
	if _, present := lookup(ExplicitKey); present {
		return refuse(stderr, "E_EXPLICIT_PRESENT", 1)
	}
	name, err := identity.Derive(ids)
	if err != nil {
		return refuse(stderr, "E_INTERNAL", 1)
	}
	if existing, present := lookup(TransportKey); present && existing != name {
		return refuse(stderr, "E_IDENTITY_CONFLICT", 1)
	}
	frame := []byte(name + "\n")
	outputStarted = true
	n, err := stdout.Write(frame)
	if err != nil || n != len(frame) {
		return refuse(stderr, "E_OUTPUT", 1)
	}
	return 0
}

// CheckFrame validates one bounded public derive frame against its expected
// name. It has no ambient lookup or operating-system dependency.
func CheckFrame(args []string, stdin io.Reader, stdout, stderr io.Writer) (exit int) {
	outputStarted := false
	defer func() {
		if recover() != nil {
			code := "E_INTERNAL"
			if outputStarted {
				code = "E_OUTPUT"
			}
			exit = refuse(stderr, code, 1)
		}
	}()

	expected, ok := parseExpectedName(args)
	if !ok {
		return refuse(stderr, "E_USAGE", 2)
	}
	if !validExpectedName(expected) {
		return refuse(stderr, "E_ID", 1)
	}

	frame, err := io.ReadAll(io.LimitReader(stdin, 71))
	if len(frame) > 70 {
		return refuse(stderr, "E_FRAME", 1)
	}
	if err != nil {
		return refuse(stderr, "E_INPUT", 1)
	}
	if len(frame) != 70 || frame[69] != '\n' || string(frame[:69]) != expected {
		return refuse(stderr, "E_FRAME", 1)
	}

	output := []byte(expected)
	outputStarted = true
	n, err := stdout.Write(output)
	if err != nil || n != len(output) {
		return refuse(stderr, "E_OUTPUT", 1)
	}
	return 0
}

func parseExpectedName(args []string) (string, bool) {
	if len(args) != 3 || args[0] != "check-frame" || args[1] != "--expected" {
		return "", false
	}
	return args[2], true
}

func validExpectedName(name string) bool {
	if len(name) != 69 || name[:5] != "syn1:" {
		return false
	}
	for i := 5; i < len(name); i++ {
		c := name[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func parse(args []string) (identity.IDs, bool) {
	var ids identity.IDs
	if len(args) != 7 || args[0] != "derive" {
		return ids, false
	}
	seen := map[string]bool{}
	for i := 1; i < len(args); i += 2 {
		flag, value := args[i], args[i+1]
		if seen[flag] {
			return ids, false
		}
		seen[flag] = true
		switch flag {
		case "--host-id":
			ids.Host = value
		case "--project-id":
			ids.Project = value
		case "--profile-id":
			ids.Profile = value
		default:
			return ids, false
		}
	}
	return ids, true
}

// A broken diagnostic sink cannot expose recovered data or cause a stack dump.
func refuse(stderr io.Writer, code string, exit int) int {
	func() {
		defer func() { _ = recover() }()
		_, _ = stderr.Write([]byte("ERR " + code + "\n"))
	}()
	return exit
}
