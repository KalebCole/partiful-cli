package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStdioWithOfficialSDK(t *testing.T) {
	binary := buildPartiful(t)
	home := t.TempDir()
	command := exec.Command(binary, "mcp", "--request-interval", "1ms")
	command.Env = append(os.Environ(), "HOME="+home)
	client := mcp.NewClient(&mcp.Implementation{Name: "partiful-test", Version: "1"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	names := make([]string, 0, len(listed.Tools))
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	wantNames := []string{
		"blasts_send",
		"cohosts_invite",
		"cohosts_link_create",
		"cohosts_link_revoke",
		"cohosts_remove",
		"cohosts_revoke_invite",
		"contacts_list",
		"events_cancel",
		"events_create",
		"events_get",
		"events_list",
		"events_update",
		"guests_invite",
		"guests_list",
		"posters_list",
		"posters_search",
		"rsvp_get",
		"rsvp_set",
	}
	slices.Sort(names)
	if !slices.Equal(names, wantNames) {
		t.Fatalf("tools = %v, want %v", names, wantNames)
	}
	read, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "events_list", Arguments: json.RawMessage(`{"when":"upcoming"}`)})
	if err != nil || !read.IsError || read.StructuredContent == nil {
		t.Fatalf("protected read = %#v, %v; want structured auth tool error", read, err)
	}
	assertMCPContentEquivalent(t, read)
	update, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "events_update",
		Arguments: json.RawMessage(
			`{"eventId":"event-example","description":null,"guestLimit":75,"links":[{"label":"Tickets","url":"https://example.test/tickets"}],"dryRun":true}`,
		),
	})
	if err != nil || !update.IsError || update.StructuredContent == nil {
		t.Fatalf("dry-run mutation = %#v, %v; want schema-valid structured auth error", update, err)
	}
	assertMCPContentEquivalent(t, update)

	for name, arguments := range map[string]json.RawMessage{
		"interested": json.RawMessage(`{"eventId":"event-example","status":"interested","dryRun":true}`),
		"going": json.RawMessage(
			`{"eventId":"event-example","status":"going","displayName":"Example Attendee","partySize":2,"plusOnes":["Guest One"],"message":null,"timezone":"UTC","questionnaireResponse":{"questionnaireVersion":0,"answers":{"question-example":"Answer"}},"dryRun":true}`,
		),
		"not-going": json.RawMessage(
			`{"eventId":"event-example","status":"not-going","displayName":"Example Attendee","partySize":1,"plusOnes":[],"message":null,"timezone":"UTC","questionnaireResponse":null,"dryRun":true}`,
		),
	} {
		t.Run(name, func(t *testing.T) {
			result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: "rsvp_set", Arguments: arguments})
			if callErr != nil || !result.IsError || result.StructuredContent == nil {
				t.Fatalf("RSVP call = %#v, %v; want schema-valid structured auth error", result, callErr)
			}
		})
	}
}

func TestMCPHelpIsLocalAndDocumentsEveryOption(t *testing.T) {
	binary := buildPartiful(t)
	const want = `Usage: partiful mcp [flags]

Runs the Partiful MCP server over stdio.

Flags:
  -h, --help                     Show help and exit (default false).
  --read-only                    Expose only read-only tools (default false).
  --allow-tool <selector>        Expose matching tools; repeat or comma-separate selectors (default all enabled tools).
  --list-tools                   Print enabled tool definitions and exit (default false).
  --timeout <duration>           Set each tool call timeout (default 30s).
  --max-concurrency <n>          Set concurrent tool call limit (default 4).
  --max-output-bytes <n>         Set encoded tool output byte limit (default 262144).
  --max-items <n>                Set per-call collection item limit (default 100).
  --request-interval <duration>  Set minimum outbound request interval (default 100ms).
`
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"help-test","version":"1"}}}` + "\n"

	for _, test := range []struct {
		name string
		argv []string
	}{
		{name: "short help", argv: []string{"-h"}},
		{name: "long help", argv: []string{"--help"}},
		{name: "help after valid flag", argv: []string{"--read-only", "--help"}},
		{name: "help before valid flag", argv: []string{"-h", "--read-only"}},
		{name: "help after unknown flag", argv: []string{"--invalid-option", "--help"}},
		{name: "help before unknown flag", argv: []string{"-h", "--invalid-option"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := exec.Command(binary, append([]string{"mcp"}, test.argv...)...)
			command.Env = append(os.Environ(), "HOME="+t.TempDir())
			command.Stdin = strings.NewReader(initialize)
			command.Stdout = &stdout
			command.Stderr = &stderr

			if err := command.Run(); err != nil {
				t.Fatalf("run help: %v\nstderr: %s", err, &stderr)
			}
			if stdout.String() != want {
				t.Fatalf("stdout = %q, want %q", stdout.String(), want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want none", &stderr)
			}
		})
	}
}

func TestMCPStdioExitsCleanlyOnEOF(t *testing.T) {
	process := startRawMCP(t, buildPartiful(t))
	stdoutDone := readAllMCPOutput(process.stdout)
	if err := process.stdin.Close(); err != nil {
		t.Fatalf("close stdin: %v", err)
	}
	if err := waitForProcess(process.command, 2*time.Second); err != nil {
		t.Fatalf("wait for EOF shutdown: %v\nstderr: %s", err, process.stderr)
	}
	stdout := <-stdoutDone
	if stdout.err != nil && !errors.Is(stdout.err, os.ErrClosed) {
		t.Fatalf("read stdout: %v", stdout.err)
	}
	if len(stdout.output) != 0 || process.stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q; want clean EOF shutdown", stdout.output, process.stderr)
	}
}

func TestMCPStdioExitsCleanlyOnSignals(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals are not supported on Windows")
	}
	for _, test := range []struct {
		name   string
		signal os.Signal
	}{
		{name: "SIGINT", signal: os.Interrupt},
		{name: "SIGTERM", signal: syscall.SIGTERM},
	} {
		t.Run(test.name, func(t *testing.T) {
			process := startInitializedMCP(t, buildPartiful(t))
			if err := process.command.Process.Signal(test.signal); err != nil {
				t.Fatalf("signal process: %v", err)
			}
			if err := waitForProcess(process.command, 2*time.Second); err != nil {
				t.Fatalf("wait for signal shutdown: %v\nstderr: %s", err, process.stderr)
			}
			if process.stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want clean signal shutdown", process.stderr)
			}
		})
	}
}

func TestMCPStdioKeepsProtocolOnStdoutAndDiagnosticsOnStderr(t *testing.T) {
	binary := buildPartiful(t)

	t.Run("protocol session", func(t *testing.T) {
		process := startInitializedMCP(t, binary)
		writeMCPMessage(t, process.stdin, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
		response := readMCPMessage(t, process.stdout)
		assertJSONRPCResponse(t, response, 2)
		stdoutDone := readAllMCPOutput(process.stdout)
		if err := process.stdin.Close(); err != nil {
			t.Fatalf("close stdin: %v", err)
		}
		if err := waitForProcess(process.command, 2*time.Second); err != nil {
			t.Fatalf("wait for protocol shutdown: %v\nstderr: %s", err, process.stderr)
		}
		remainder := <-stdoutDone
		if remainder.err != nil && !errors.Is(remainder.err, os.ErrClosed) {
			t.Fatalf("read remaining stdout: %v", remainder.err)
		}
		if len(bytes.TrimSpace(remainder.output)) != 0 {
			t.Fatalf("non-protocol stdout after response: %q", remainder.output)
		}
		if process.stderr.Len() != 0 {
			t.Fatalf("stderr = %q, want no diagnostics for a healthy session", process.stderr)
		}
	})

	t.Run("startup diagnostic", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		command := exec.Command(binary, "mcp", "--invalid-option")
		command.Env = append(os.Environ(), "HOME="+t.TempDir())
		command.Stdout = &stdout
		command.Stderr = &stderr
		err := command.Run()
		exitError, ok := err.(*exec.ExitError)
		if !ok || exitError.ExitCode() != 2 {
			t.Fatalf("run error = %v, want exit code 2", err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout = %q, want diagnostics confined to stderr", stdout.String())
		}
		if !strings.Contains(stderr.String(), "partiful mcp: unknown mcp option") {
			t.Fatalf("stderr = %q, want startup diagnostic", stderr.String())
		}
	})
}

type rawMCPProcess struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	stderr  *bytes.Buffer
}

type mcpOutputRead struct {
	output []byte
	err    error
}

func buildPartiful(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "partiful")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return binary
}

func startRawMCP(t *testing.T, binary string) *rawMCPProcess {
	t.Helper()
	command := exec.Command(binary, "mcp", "--request-interval", "1ms")
	command.Env = append(os.Environ(), "HOME="+t.TempDir())
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr := &bytes.Buffer{}
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		t.Fatalf("start MCP process: %v", err)
	}
	t.Cleanup(func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	})
	return &rawMCPProcess{
		command: command,
		stdin:   stdin,
		stdout:  bufio.NewReader(stdout),
		stderr:  stderr,
	}
}

func startInitializedMCP(t *testing.T, binary string) *rawMCPProcess {
	t.Helper()
	process := startRawMCP(t, binary)
	writeMCPMessage(t, process.stdin, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"lifecycle-test","version":"1"}}}`)
	assertJSONRPCResponse(t, readMCPMessage(t, process.stdout), 1)
	writeMCPMessage(t, process.stdin, `{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}`)
	return process
}

func writeMCPMessage(t *testing.T, writer io.Writer, message string) {
	t.Helper()
	if _, err := io.WriteString(writer, message+"\n"); err != nil {
		t.Fatalf("write MCP message: %v", err)
	}
}

func readMCPMessage(t *testing.T, reader *bufio.Reader) []byte {
	t.Helper()
	type readResult struct {
		message []byte
		err     error
	}
	result := make(chan readResult, 1)
	go func() {
		message, err := reader.ReadBytes('\n')
		result <- readResult{message: bytes.TrimSpace(message), err: err}
	}()
	select {
	case read := <-result:
		if read.err != nil {
			t.Fatalf("read MCP message: %v", read.err)
		}
		if !json.Valid(read.message) {
			t.Fatalf("stdout line is not JSON-RPC: %q", read.message)
		}
		return read.message
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for MCP message")
		return nil
	}
}

func readAllMCPOutput(reader io.Reader) <-chan mcpOutputRead {
	result := make(chan mcpOutputRead, 1)
	go func() {
		output, err := io.ReadAll(reader)
		result <- mcpOutputRead{output: output, err: err}
	}()
	return result
}

func assertJSONRPCResponse(t *testing.T, message []byte, id int) {
	t.Helper()
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error"`
	}
	if err := json.Unmarshal(message, &response); err != nil {
		t.Fatalf("decode JSON-RPC response: %v", err)
	}
	if response.JSONRPC != "2.0" || response.ID != id || len(response.Result) == 0 || len(response.Error) != 0 {
		t.Fatalf("response = %s, want successful JSON-RPC id %d", message, id)
	}
}

func waitForProcess(command *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() {
		done <- command.Wait()
	}()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = command.Process.Kill()
		<-done
		return fmt.Errorf("process did not exit within %s", timeout)
	}
}

func assertMCPContentEquivalent(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v, want one text envelope", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want text", result.Content[0])
	}
	var textEnvelope any
	if err := json.Unmarshal([]byte(text.Text), &textEnvelope); err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if !reflect.DeepEqual(textEnvelope, result.StructuredContent) {
		t.Fatalf("text envelope = %#v, structured envelope = %#v", textEnvelope, result.StructuredContent)
	}
}
