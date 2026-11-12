package main

import (
	"io"
	"os"
	"syscall"

	"github.com/relux-works/swarma-launch-identity/internal/cli"
)

func main() {
	os.Exit(runCommand(os.Args[1:], os.LookupEnv, os.Stdin, outputWriter{}, os.Stderr))
}

func runCommand(args []string, lookup cli.Lookup, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "check-frame" {
		return cli.CheckFrame(args, stdin, stdout, stderr)
	}
	return cli.Run(args, lookup, stdout, stderr)
}

// Direct stdout writes return EPIPE to the command instead of the special
// os.File stdout behavior that terminates the process with SIGPIPE.
type outputWriter struct{}

func (outputWriter) Write(p []byte) (int, error) {
	return syscall.Write(syscall.Stdout, p)
}
