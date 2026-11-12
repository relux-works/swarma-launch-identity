package examples

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const callerName = "syn1:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type callerCase struct {
	name            string
	hostID          string
	projectID       string
	profileID       string
	producerFrame   string
	producerExit    int
	filterMode      string
	expectedName    string
	readonlyCleanup bool
	wantLaunch      bool
}

type callerResult struct {
	exit     int
	stdout   []byte
	stderr   []byte
	starts   []byte
	identity []byte
	agent    []byte
	state    []byte
	producer []string
	args     []string
}

// This table proves one exact success and fail-closed behavior for every
// producer, frame, filter, mapping, and cleanup refusal; each refusal runs
// beside a fresh successful receiver control.
func TestCallerTemplateGates(t *testing.T) {
	cases := []callerCase{
		{name: "valid frame starts one receiver", wantLaunch: true},
		{name: "shell metacharacters and Unicode IDs stay literal", hostID: "host ' $ `", projectID: "e\u0301", profileID: "profile with spaces", wantLaunch: true},
		{name: "producer nonzero with valid output", producerExit: 7},
		{name: "malformed frame", producerFrame: "not-a-frame\n"},
		{name: "filter failure", filterMode: "fail"},
		{name: "filter partial output and failure", filterMode: "partial"},
		{name: "expected name mismatch", expectedName: "syn1:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{name: "cleanup failure", readonlyCleanup: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.wantLaunch {
				control := runCaller(t, callerCase{wantLaunch: true})
				assertCallerSuccess(t, control, callerCase{})
			}
			got := runCaller(t, tc)
			if tc.wantLaunch {
				assertCallerSuccess(t, got, tc)
				return
			}
			if got.exit == 0 || len(got.stdout) != 0 || len(got.stderr) != 0 || len(got.starts) != 0 || string(got.state) != "absent" {
				t.Fatal("refusal started a receiver, exported an identity, or leaked pipeline output")
			}
		})
	}
}

// This is the required regression witness: pipefail must preserve an earlier
// producer failure even when the filter emits a valid-looking name.
func TestCallerRejectsNonzeroProducerWithValidFrame(t *testing.T) {
	got := runCaller(t, callerCase{producerExit: 7})
	if got.exit == 0 || len(got.starts) != 0 || string(got.state) != "absent" {
		t.Fatal("nonzero producer was accepted through a successful filter")
	}
}

// The template cleans exactly the documented variables and preserves the
// transport key for derive to validate; it also keeps pipefail and cd refusal.
func TestTemplateCleanupScope(t *testing.T) {
	data, err := os.ReadFile("review-seat-launch.zsh.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == "unset" {
			got = append(got, fields[1])
		}
	}
	want := []string{
		"CODEX_HOME", "CODEX_THREAD_ID", "CLAUDE_CONFIG_DIR", "CLAUDECODE",
		"CLAUDE_CODE_ENTRYPOINT", "A2A_SESSION_ID", "A2A_PROFILE", "A2A_CONNECTION",
		"A2A_CONNECTION_FINGERPRINT", "A2A_AGENT", "A2A_CONFIG_DIR",
		"ANTHROPIC_API_KEY", "TASK_BOARD_RUN_ID",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") ||
		!strings.Contains(string(data), "setopt pipefail") ||
		!strings.Contains(string(data), "env -i") || !strings.Contains(string(data), "tmux") ||
		!strings.Contains(string(data), `cd __SEAT_DIRECTORY__ || exit 1`) ||
		strings.Contains(string(data), "unset SWARMA_LAUNCH_IDENTITY") {
		t.Fatal("template cleanup or pipeline boundary changed")
	}
	header := strings.SplitN(string(data), "launch_review_seat() {", 2)[0]
	for _, placeholder := range []string{
		"__HOST_ID__", "__PROJECT_ID__", "__PROFILE_ID__", "__EXPECTED_NAME__",
		"__UTILITY_PATH__", "__SEAT_DIRECTORY__", "__PROMPT_FILE__",
		"__CURATOR_COMMAND__", "__CURATOR_ARGUMENTS__",
	} {
		if !strings.Contains(header, placeholder) {
			t.Fatalf("template header does not document %s", placeholder)
		}
	}
}

func runCaller(t *testing.T, tc callerCase) callerResult {
	t.Helper()
	if tc.hostID == "" {
		tc.hostID = "fixture-host"
	}
	if tc.projectID == "" {
		tc.projectID = "fixture-project"
	}
	if tc.profileID == "" {
		tc.profileID = "fixture-profile"
	}
	if tc.producerFrame == "" {
		tc.producerFrame = callerName + "\n"
	}
	if tc.expectedName == "" {
		tc.expectedName = callerName
	}
	if tc.filterMode == "" {
		tc.filterMode = "accept"
	}

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seatDir := filepath.Join(root, "seat's directory")
	if err := os.Mkdir(seatDir, 0700); err != nil {
		t.Fatal(err)
	}
	promptPath := filepath.Join(root, "prompt's file.txt")
	if err := os.WriteFile(promptPath, []byte("fixture prompt\n\n"), 0600); err != nil {
		t.Fatal(err)
	}

	producerPath := filepath.Join(root, "producer's script.zsh")
	producerArgsPath := filepath.Join(root, "producer-args")
	producerScript := "#!/bin/zsh -f\n" +
		"if (( ${+SWARMA_LAUNCH_IDENTITY} || ${+A2A_AGENT} )); then exit 97; fi\n" +
		"printf '%s\\0' \"$@\" > " + zshQuote(producerArgsPath) + "\n" +
		"printf '%s' " + zshQuote(tc.producerFrame) + "\n"
	if tc.producerExit != 0 {
		producerScript += "printf '%s\\n' 'fixture producer stderr' >&2\n"
	}
	producerScript += "exit " + itoa(tc.producerExit) + "\n"
	writeExecutable(t, producerPath, producerScript)

	filterPath := filepath.Join(root, "filter's script.zsh")
	filterScript := filterStub(tc.filterMode)
	writeExecutable(t, filterPath, filterScript)

	utilityPath := filepath.Join(root, "utility's script.zsh")
	utilityScript := "#!/bin/zsh -f\n" +
		"if [[ \"$1\" == derive ]]; then exec " + zshQuote(producerPath) + " \"$@\"; fi\n" +
		"if [[ \"$1\" == check-frame ]]; then exec " + zshQuote(filterPath) + " \"$@\"; fi\n" +
		"exit 2\n"
	writeExecutable(t, utilityPath, utilityScript)

	startsPath := filepath.Join(root, "starts")
	identityPath := filepath.Join(root, "identity value")
	agentPath := filepath.Join(root, "agent value")
	argsPath := filepath.Join(root, "receiver args")
	receiverPath := filepath.Join(root, "receiver's script.zsh")
	receiverScript := "#!/bin/zsh -f\n" +
		"print -rn -- x >> " + zshQuote(startsPath) + "\n" +
		"if (( ${+SWARMA_LAUNCH_IDENTITY} )); then print -rn -- \"$SWARMA_LAUNCH_IDENTITY\" > " + zshQuote(identityPath) + "; else print -rn -- absent > " + zshQuote(identityPath) + "; fi\n" +
		"if (( ${+A2A_AGENT} )); then print -rn -- present > " + zshQuote(agentPath) + "; else print -rn -- absent > " + zshQuote(agentPath) + "; fi\n" +
		"printf '%s\\0' \"$@\" > " + zshQuote(argsPath) + "\n"
	writeExecutable(t, receiverPath, receiverScript)

	templateBytes, err := os.ReadFile("review-seat-launch.zsh.tmpl")
	if err != nil {
		t.Fatal(err)
	}
	replacements := strings.NewReplacer(
		"__HOST_ID__", zshQuote(tc.hostID),
		"__PROJECT_ID__", zshQuote(tc.projectID),
		"__PROFILE_ID__", zshQuote(tc.profileID),
		"__EXPECTED_NAME__", zshQuote(tc.expectedName),
		"__UTILITY_PATH__", zshQuote(utilityPath),
		"__SEAT_DIRECTORY__", zshQuote(seatDir),
		"__PROMPT_FILE__", zshQuote(promptPath),
		"__CURATOR_COMMAND__", zshQuote(receiverPath),
		"__CURATOR_ARGUMENTS__", `'--fixture-mode' 'inert'`,
	)
	rendered := replacements.Replace(string(templateBytes))
	if strings.Contains(rendered, "__") {
		t.Fatal("caller template contains an unfilled placeholder")
	}
	templatePath := filepath.Join(root, "rendered.zsh")
	if err := os.WriteFile(templatePath, []byte(rendered), 0600); err != nil {
		t.Fatal(err)
	}

	statePath := filepath.Join(root, "identity-state")
	trapBody := "if (( ${+SWARMA_LAUNCH_IDENTITY} )); then print -rn -- present > " + zshQuote(statePath) +
		"; else print -rn -- absent > " + zshQuote(statePath) + "; fi"
	runner := "#!/bin/zsh -f\n"
	if tc.readonlyCleanup {
		runner += "readonly CODEX_HOME=fixture\n"
	}
	runner += "trap " + zshQuote(trapBody) + " EXIT\nsource " + zshQuote(templatePath) + "\n"
	runnerPath := filepath.Join(root, "runner.zsh")
	if err := os.WriteFile(runnerPath, []byte(runner), 0600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("/bin/zsh", "-f", runnerPath)
	cmd.Dir = root
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	exit := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(runErr, &exitErr) {
			t.Fatalf("zsh could not run fixture: %v", runErr)
		}
		exit = exitErr.ExitCode()
	}
	return callerResult{
		exit: exit, stdout: stdout.Bytes(), stderr: stderr.Bytes(),
		starts: readFixtureFile(t, startsPath), identity: readFixtureFile(t, identityPath),
		agent: readFixtureFile(t, agentPath), state: readFixtureFile(t, statePath),
		producer: splitNUL(readFixtureFile(t, producerArgsPath)),
		args:     splitNUL(readFixtureFile(t, argsPath)),
	}
}

func filterStub(mode string) string {
	script := "#!/bin/zsh -f\n" +
		"if (( ${+SWARMA_LAUNCH_IDENTITY} || ${+A2A_AGENT} )); then exit 97; fi\n" +
		"input=$(cat; printf '.')\ninput=${input%.}\n" +
		"if [[ \"$3\" != " + zshQuote(callerName) + " || \"$input\" != " + zshQuote(callerName) + "$'\\n' ]]; then exit 1; fi\n"
	switch mode {
	case "partial":
		return script + "printf '%s' " + zshQuote(callerName[:12]) + "\nprintf '%s\\n' 'fixture filter stderr' >&2\nexit 1\n"
	case "fail":
		return script + "printf '%s\\n' 'fixture filter stderr' >&2\nexit 1\n"
	default:
		return script + "printf '%s' " + zshQuote(callerName) + "\nexit 0\n"
	}
}

func assertCallerSuccess(t *testing.T, got callerResult, tc callerCase) {
	t.Helper()
	if tc.hostID == "" {
		tc.hostID = "fixture-host"
	}
	if tc.projectID == "" {
		tc.projectID = "fixture-project"
	}
	if tc.profileID == "" {
		tc.profileID = "fixture-profile"
	}
	if got.exit != 0 || len(got.stdout) != 0 || len(got.stderr) != 0 || string(got.starts) != "x" ||
		string(got.identity) != callerName || string(got.agent) != "absent" || string(got.state) != "" ||
		strings.Join(got.producer, "|") != "derive|--host-id|"+tc.hostID+"|--project-id|"+tc.projectID+"|--profile-id|"+tc.profileID ||
		strings.Join(got.args, "|") != "--fixture-mode|inert|fixture prompt" {
		t.Fatal("validated frame did not reach exactly one inert receiver with the exact identity and argv")
	}
}

func writeExecutable(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0700); err != nil {
		t.Fatal(err)
	}
}

func readFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func splitNUL(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	parts := strings.Split(string(b), "\x00")
	return parts[:len(parts)-1]
}

func zshQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + itoa(-n)
	}
	var digits [20]byte
	i := len(digits)
	for n > 0 {
		i--
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits[i:])
}
