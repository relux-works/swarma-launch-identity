# swarma-launch-identity

Compute a deterministic public selector from explicitly assigned stable host,
project and logical profile IDs. Directory moves, profile versions and process
restarts do not change a retained tuple. The command creates no account, key,
credential, session, home or token, starts no child, and writes no file.

Requires Go 1.25; standard library only.

```sh
go build -o bin/swarma-launch-identity ./cmd/swarma-launch-identity
bin/swarma-launch-identity derive --host-id h --project-id p --profile-id r
```

The example exits 0, leaves stderr empty and writes exactly this line with one
LF (70 bytes total):

```text
syn1:dabc9bcf3b62c00fb8cefde216710dbae06b8e96bef04b8cdde47cf3f4e746f3
```

Supply exactly one of each separated flag; their order is arbitrary. Values
are opaque, nonempty valid UTF-8, at most 256 bytes each, without Unicode
control characters. Empty supplied values refuse E_ID. There is no trimming,
case folding, Unicode normalization or path cleaning. The digest is SHA-256 of
ASCII `swarma.synthetic-identity/v1`, a NUL, then host, project and profile,
each prefixed by its UTF-8 byte length as a uint32 in big-endian order. The
name is `syn1:` plus 64 lowercase hexadecimal digits. Public literal vectors
are in `testdata/public-encoding-vectors-r0.json`.

The pure Go API is `identity.IDs{Host: ..., Project: ..., Profile: ...}` with
`Validate`, `identity.Encode` and `identity.Derive`. It performs no I/O.

| Refusal | Exit | Trigger |
| --- | --- | --- |
| E_USAGE | 2 | Wrong command, missing/duplicate/unknown flag, missing value or positional form |
| E_ID | 1 | Empty, invalid UTF-8, control-containing or overlong ID |
| E_EXPLICIT_PRESENT | 1 | A2A_AGENT exists, including an empty value |
| E_IDENTITY_CONFLICT | 1 | SWARMA_LAUNCH_IDENTITY exists and is empty or differs from the derived name |
| E_OUTPUT | 1 | Output write fails, is short, or unexpectedly fails after output starts |
| E_INTERNAL | 1 | Unexpected failure before output starts |

Evaluation order is command shape, all ID representation checks, explicit
identity presence, derivation, existing transport comparison, output. A refusal
writes only `ERR CODE` and LF to stderr. Stdout stays empty before output
begins. The command attempts one bounded output write; a failure may leave a
partial frame. Discard all stdout unless the process exits 0 and the complete
frame validates exactly; never launch from failed, partial or trailing output.

Only A2A_AGENT and SWARMA_LAUNCH_IDENTITY are read from the environment. Neither
is changed. An absent or equal transport value succeeds. Explicit identity
launches retain their separate existing route: never put a synthetic name into
A2A_AGENT, strip its prefix, enroll it, or enable another identity's hooks.

The selector has the existing same-UID trust limit: it authenticates nobody
and grants no credential access. Caller configuration owns the stable public
ID mapping and its uniqueness; the utility cannot verify provenance or global
uniqueness, and collision resistance does not prove all tuples have distinct
names. Missing assignments must never fall back to hostname, path, PID or
profile-version guesses.

Before real adoption, the launching manager must qualify its selected no-proxy
Codex route through the admitted Curator launch command. Capture the complete
frame, reserve SWARMA_LAUNCH_IDENTITY, review the nonsecret key-name/selection
projection for later overlays and auth-token-name pointers, pin the route and
configuration, and prove exact bytes at an inert child. Existing composition
may warn and overwrite an overlay; the exact-value qualification checker must
reject that change. Other routes require separate qualification. Bridge success
alone proves no receiving-route transport, namespace separation, home writing,
atomicity, resume persistence, enrollment or credential release.

## Tools

Run test suites serially. Before every test invocation, `pgrep -x Runner.Worker`
must print nothing. If it prints a PID, wait two minutes and check again before
running tests. Never stop another process to clear this guard.

`check-frame` validates raw `derive` output against a separately reviewed
expected name. Run exactly:

```sh
bin/swarma-launch-identity check-frame --expected NAME
```

`NAME` is `syn1:` plus 64 lowercase hexadecimal digits. The command reads at
most 71 bytes and accepts only `NAME` plus one LF (70 bytes), followed by EOF.
Success exits 0, writes `NAME` without LF (69 bytes), and leaves stderr empty.
Refusals write fixed `ERR CODE` diagnostics: E_USAGE/2 for command-shape
errors; E_ID, E_INPUT, E_FRAME, E_OUTPUT, or E_INTERNAL/1 for malformed
expected names, input failures, frame mismatches, output failures, or
unexpected pre-output panics. Other than E_OUTPUT, refusals leave stdout empty.
A short or failed output write may leave partial bytes on stdout; discard all
captured output on any nonzero exit. The command does not read the environment
or access files, children, networks, credentials, or homes.

The reviewed example `examples/review-seat-launch.zsh.tmpl` shows a manager
capturing `derive | check-frame` in a clean `env -i` tmux launch, then exporting
the validated public name before its configured Curator command. A manager
copies the example and fills its documented placeholders with shell-quoted
literal IDs, expected name, utility path, seat and prompt paths, and command
path and arguments. The example and its fixture do not qualify a real Curator
route.

| Tool and purpose | Exact command | Output location |
| --- | --- | --- |
| Go compiler: build the CLI | `go build -o bin/swarma-launch-identity ./cmd/swarma-launch-identity` | `bin/swarma-launch-identity` |
| Frame checker: validate one exact public frame | `bin/swarma-launch-identity check-frame --expected NAME` | Validated name on stdout without LF |
| Caller example: reviewed zsh launch template | `examples/review-seat-launch.zsh.tmpl` | Copy and fill its documented placeholders in the manager-owned launch configuration |
| Go vet: static checks | `GOFLAGS=-mod=mod GOPROXY=off go vet ./...` | Diagnostics on terminal stderr |
| Go tests: serial contract and race checks | `GOPROXY=off go test -p 1 -race -count=1 ./...` | Verdicts on terminal stdout/stderr |
| Go formatter: format sources | `gofmt -w identity.go identity_test.go resources_test.go internal/cli/cli.go internal/cli/cli_test.go cmd/swarma-launch-identity/main.go cmd/swarma-launch-identity/main_test.go` | Listed source files |

Tests pin bytes and names, validate each refusal with valid controls, check
precedence and byte limits, bound `check-frame` input to 71 bytes, instrument
the derive environment reads and bounded output attempts, plant diagnostic and
forbidden-lookup controls, constrain production imports, and exercise process
frames in disposable directories. These checks do not execute a real launcher
or qualify a manager configuration.

Licensed under Apache-2.0; see LICENSE and NOTICE.
