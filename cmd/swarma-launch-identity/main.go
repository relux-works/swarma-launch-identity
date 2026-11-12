package main

import (
	"os"
	"syscall"

	"github.com/relux-works/swarma-launch-identity/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.LookupEnv, outputWriter{}, os.Stderr))
}

// Direct stdout writes return EPIPE to the command instead of the special
// os.File stdout behavior that terminates the process with SIGPIPE.
type outputWriter struct{}

func (outputWriter) Write(p []byte) (int, error) {
	return syscall.Write(syscall.Stdout, p)
}
