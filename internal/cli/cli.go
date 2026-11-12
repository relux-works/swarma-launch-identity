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
