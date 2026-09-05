package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerRejectsSchemaInvalidSuccessfulOutput(t *testing.T) {
	privateOutput := "private-output-marker"
	server, err := newServer(app.Dependencies{}, Options{MaxBytes: 256}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{Stdout: `{"ok":true,"data":{"items":[{"posterId":42,"diagnostic":"` + privateOutput + `"}]},"meta":{"command":"posters.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":[]}}` + "\n"}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{},
	})
	if callErr != nil {
		t.Fatalf("schema-invalid output returned protocol error: %v", callErr)
	}
	assertToolErrorCode(t, result, "MCP_OUTPUT_INVALID")
	assertToolTextWithinLimit(t, result, 256)
	visible, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(visible), privateOutput) {
		t.Fatal("output-schema validation error disclosed invalid application output")
	}
}

func TestServerValidatesInputBeforeInvocation(t *testing.T) {
	calls := 0
	server, err := newServer(app.Dependencies{}, Options{MaxBytes: 256}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		calls++
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{"limit": "not-an-integer"},
	})
	if callErr != nil {
		t.Fatalf("schema-invalid input returned protocol error: %v", callErr)
	}
	if calls != 0 {
		t.Fatalf("invocation calls = %d, want zero; call error = %v", calls, callErr)
	}
	assertToolErrorCode(t, result, "MCP_ARGUMENTS_INVALID")
	assertToolTextWithinLimit(t, result, 256)
}

func TestServerPreservesLargeIntegerInputBeforeInvocation(t *testing.T) {
	const guestLimit int64 = 1<<53 + 1
	var captured any
	server, err := newServer(app.Dependencies{}, Options{}, func(
		_ context.Context,
		_ string,
		arguments map[string]any,
		_ app.Dependencies,
		_ ...app.MCPExecutionOptions,
	) app.Result {
		captured = arguments["guestLimit"]
		return app.Result{
			Stdout:   `{"ok":false,"error":{"type":"input.invalid","code":"TEST_STOP","message":"Stop after capture.","retryable":false,"details":{}}}`,
			ExitCode: 2,
		}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "events_create",
		Arguments: map[string]any{
			"title":      "Example event",
			"start":      "2026-09-12T19:00:00Z",
			"timezone":   "UTC",
			"guestLimit": guestLimit,
			"dryRun":     true,
		},
	})
	if callErr != nil || result == nil || !result.IsError {
		t.Fatalf("result = %#v, error = %v; want captured application error", result, callErr)
	}
	if captured != guestLimit {
		t.Fatalf("captured guestLimit = %#v, want exact int64 %d", captured, guestLimit)
	}
}

func TestServerRejectsNearIntegerFractionBeforeInvocation(t *testing.T) {
	calls := 0
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		calls++
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: json.RawMessage(`{"limit":1.` + "00000000" + "00000001" + `}`),
	})
	if callErr != nil {
		t.Fatalf("schema-invalid input returned protocol error: %v", callErr)
	}
	if calls != 0 {
		t.Fatalf("invocation calls = %d, want zero", calls)
	}
	assertToolErrorCode(t, result, "MCP_ARGUMENTS_INVALID")
}

func TestServerRejectsLargeFractionBeforeInvocation(t *testing.T) {
	calls := 0
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		calls++
		return app.Result{
			Stdout:   `{"ok":false,"error":{"type":"input.invalid","code":"TEST_STOP","message":"Stop after capture.","retryable":false,"details":{}}}`,
			ExitCode: 2,
		}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "events_create",
		Arguments: json.RawMessage(
			`{"title":"Example event","start":"2026-09-12T19:00:00Z","timezone":"UTC","guestLimit":` +
				"900719925" + "4740992.1" + `,"dryRun":true}`,
		),
	})
	if callErr != nil {
		t.Fatalf("schema-invalid input returned protocol error: %v", callErr)
	}
	if calls != 0 {
		t.Fatalf("invocation calls = %d, want zero", calls)
	}
	assertToolErrorCode(t, result, "MCP_ARGUMENTS_INVALID")
}

func TestServerReturnsStructuredRedactedInputValidationErrors(t *testing.T) {
	calls := 0
	var diagnostics bytes.Buffer
	server, err := newServerWithSDKOptions(app.Dependencies{}, Options{MaxBytes: 256}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		calls++
		return validPosterResult()
	}, &mcp.ServerOptions{
		Logger: slog.New(slog.NewJSONHandler(&diagnostics, nil)),
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)
	privateMessage := "private-message-marker-" + strings.Repeat("x", 2000)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "blasts_send",
		Arguments: map[string]any{
			"eventId":  "event-example",
			"audience": "all-guests",
			"message":  privateMessage,
		},
	})
	if callErr != nil {
		t.Fatalf("schema-invalid call returned protocol error: %v", callErr)
	}
	if calls != 0 {
		t.Fatalf("invocation calls = %d, want zero", calls)
	}
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("result = %#v, want one structured tool error", result)
	}
	visible, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for surface, value := range map[string]string{
		"tool result":    string(visible),
		"protocol error": errorText(callErr),
		"diagnostics":    diagnostics.String(),
	} {
		if strings.Contains(value, privateMessage) {
			t.Fatalf("schema validation error disclosed the private blast message in %s", surface)
		}
	}
	assertToolErrorCode(t, result, "MCP_ARGUMENTS_INVALID")
	assertToolTextWithinLimit(t, result, 256)
}

func TestServerPublishesExactInputAndOutputSchemas(t *testing.T) {
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)
	listed, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}

	definitions := make(map[string]app.MCPDefinition)
	for _, definition := range app.MCPDefinitions() {
		definitions[definition.Name] = definition
	}
	if len(listed.Tools) != len(definitions) {
		t.Fatalf("listed tools = %d, want %d", len(listed.Tools), len(definitions))
	}
	for _, tool := range listed.Tools {
		definition, ok := definitions[tool.Name]
		if !ok {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		assertSameJSON(t, tool.InputSchema, definition.InputSchema)
		assertSameJSON(t, tool.OutputSchema, definition.OutputSchema)
	}
}

func TestServerTextAndStructuredContentAreTheSameEnvelope(t *testing.T) {
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatalf("content = %#v", result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want text", result.Content[0])
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var textEnvelope, structuredEnvelope any
	if err := json.Unmarshal([]byte(text.Text), &textEnvelope); err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if err := json.Unmarshal(structured, &structuredEnvelope); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if !reflect.DeepEqual(textEnvelope, structuredEnvelope) {
		t.Fatalf("text = %s, structured = %s", text.Text, structured)
	}
}

func TestServerPacesOutboundRequests(t *testing.T) {
	var (
		mutex sync.Mutex
		times []time.Time
	)
	dependencies := app.Dependencies{HTTP: httpClientFunc(func(*http.Request) (*http.Response, error) {
		mutex.Lock()
		times = append(times, time.Now())
		mutex.Unlock()
		return nil, nil
	})}
	server, err := newServer(dependencies, Options{
		RequestInterval: 75 * time.Millisecond,
	}, func(
		ctx context.Context,
		_ string,
		_ map[string]any,
		dependencies app.Dependencies,
		_ ...app.MCPExecutionOptions,
	) app.Result {
		for range 2 {
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
			if requestErr != nil {
				t.Fatalf("new request: %v", requestErr)
			}
			if _, requestErr := dependencies.HTTP.Do(request); requestErr != nil {
				t.Fatalf("paced request: %v", requestErr)
			}
		}
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	if _, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{},
	}); err != nil {
		t.Fatalf("call tool: %v", err)
	}

	mutex.Lock()
	defer mutex.Unlock()
	if len(times) != 2 || times[1].Sub(times[0]) < 60*time.Millisecond {
		t.Fatalf("invocation times = %v, want at least 60ms spacing", times)
	}
}

func TestServerReturnsBoundedErrorInsteadOfOversizedOutput(t *testing.T) {
	server, err := newServer(app.Dependencies{}, Options{
		MaxBytes:        256,
		RequestInterval: time.Nanosecond,
	}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{Stdout: `{"ok":true,"data":{"items":[]},"meta":{"command":"posters.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":["` + strings.Repeat("x", 300) + `"]}}`}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	assertToolErrorCode(t, result, "MCP_OUTPUT_LIMIT")
	text := result.Content[0].(*mcp.TextContent).Text
	if len(text) > 256 {
		t.Fatalf("bounded error length = %d, want at most 256", len(text))
	}
}

func TestServerBoundsMutationOutputWithoutInvitingRetry(t *testing.T) {
	server, err := newServer(app.Dependencies{}, Options{
		MaxBytes:        256,
		RequestInterval: time.Nanosecond,
	}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{
			Stdout: `{"ok":false,"error":{"type":"remote.unavailable","code":"EVENT_SUBMISSION_UNCERTAIN","message":"` +
				strings.Repeat("x", 300) +
				`","retryable":false,"details":{}}}`,
			ExitCode: 8,
		}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "events_cancel",
		Arguments: map[string]any{
			"eventId": "event-example",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	assertToolErrorCode(t, result, "MCP_MUTATION_OUTCOME_UNCERTAIN")
	text := result.Content[0].(*mcp.TextContent).Text
	if len(text) > 256 {
		t.Fatalf("bounded mutation error length = %d, want at most 256", len(text))
	}
	var envelope struct {
		Error struct {
			Retryable bool `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(text), &envelope); err != nil {
		t.Fatalf("decode mutation error: %v", err)
	}
	if envelope.Error.Retryable {
		t.Fatal("bounded mutation error is retryable; want uncertain non-retryable outcome")
	}
}

func TestServerMeasuresTransmittedOutputWithoutTrailingNewline(t *testing.T) {
	const (
		prefix = `{"ok":true,"data":{"items":[]},"meta":{"command":"posters.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":["`
		suffix = `"]}}`
	)
	body := prefix + strings.Repeat("x", 256-len(prefix)-len(suffix)) + suffix
	if len(body) != 256 {
		t.Fatalf("fixture length = %d, want 256", len(body))
	}
	server, err := newServer(app.Dependencies{}, Options{
		MaxBytes:        len(body),
		RequestInterval: time.Nanosecond,
	}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{Stdout: body + "\n"}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{},
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("result = %#v, error = %v; want success at exact byte limit", result, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Text != body {
		t.Fatalf("transmitted text = %#v, want exact application body", result.Content)
	}
}

func TestServerPreservesValidatedOutputNumberLexemes(t *testing.T) {
	const body = `{"ok":true,"data":{"items":[{"displayName":"Example Contact","sharedEventCount":` +
		"999999999" + "999999999" +
		`}]},"meta":{"command":"contacts.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":[]}}`
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{Stdout: body + "\n"}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "contacts_list",
		Arguments: map[string]any{},
	})
	if err != nil || result == nil || result.IsError {
		t.Fatalf("result = %#v, error = %v; want success", result, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || text.Text != body {
		t.Fatalf("transmitted text = %#v, want exact application number", result.Content)
	}
}

func TestServerRejectsNearIntegerFractionalOutput(t *testing.T) {
	const body = `{"ok":true,"data":{"items":[{"displayName":"Example Contact","sharedEventCount":1.` +
		"00000000" + "00000001" +
		`}]},"meta":{"command":"contacts.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":[]}}`
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{Stdout: body + "\n"}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "contacts_list",
		Arguments: map[string]any{},
	})
	if callErr != nil {
		t.Fatalf("schema-invalid output returned protocol error: %v", callErr)
	}
	assertToolErrorCode(t, result, "MCP_OUTPUT_INVALID")
}

func TestServerRejectsLargeFractionalOutput(t *testing.T) {
	const body = `{"ok":true,"data":{"items":[{"displayName":"Example Contact","sharedEventCount":` +
		"900719925" + "4740992.1" +
		`}]},"meta":{"command":"contacts.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":[]}}`
	server, err := newServer(app.Dependencies{}, Options{}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return app.Result{Stdout: body + "\n"}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "contacts_list",
		Arguments: map[string]any{},
	})
	if callErr != nil {
		t.Fatalf("schema-invalid output returned protocol error: %v", callErr)
	}
	assertToolErrorCode(t, result, "MCP_OUTPUT_INVALID")
}

func TestServerEnforcesCallTimeout(t *testing.T) {
	server, err := newServer(app.Dependencies{}, Options{
		CallTimeout:     25 * time.Millisecond,
		RequestInterval: time.Nanosecond,
	}, func(
		ctx context.Context,
		_ string,
		_ map[string]any,
		_ app.Dependencies,
		_ ...app.MCPExecutionOptions,
	) app.Result {
		<-ctx.Done()
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "posters_list",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	assertToolErrorCode(t, result, "MCP_CALL_TIMEOUT")
}

func TestServerPreservesMutationOutcomeAfterDeadline(t *testing.T) {
	server, err := newServer(app.Dependencies{}, Options{
		CallTimeout:     25 * time.Millisecond,
		RequestInterval: time.Nanosecond,
	}, func(
		ctx context.Context,
		_ string,
		_ map[string]any,
		_ app.Dependencies,
		_ ...app.MCPExecutionOptions,
	) app.Result {
		<-ctx.Done()
		return app.Result{
			Stdout:   `{"ok":false,"error":{"type":"remote.unavailable","code":"EVENT_SUBMISSION_UNCERTAIN","message":"Submission may have completed; inspect remote state before retrying.","retryable":false,"details":{}}}`,
			ExitCode: 8,
		}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "events_cancel",
		Arguments: map[string]any{
			"eventId": "event-example",
		},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	assertToolErrorCode(t, result, "EVENT_SUBMISSION_UNCERTAIN")
}

func TestServerPropagatesCallCancellation(t *testing.T) {
	started := make(chan struct{})
	cancelled := make(chan error, 1)
	server, err := newServer(app.Dependencies{}, Options{
		RequestInterval: time.Nanosecond,
	}, func(
		ctx context.Context,
		_ string,
		_ map[string]any,
		_ app.Dependencies,
		_ ...app.MCPExecutionOptions,
	) app.Result {
		close(started)
		<-ctx.Done()
		cancelled <- ctx.Err()
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)
	ctx, cancel := context.WithCancel(context.Background())
	callDone := make(chan error, 1)
	go func() {
		_, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "posters_list",
			Arguments: map[string]any{},
		})
		callDone <- callErr
	}()

	<-started
	cancel()
	select {
	case observed := <-cancelled:
		if !errors.Is(observed, context.Canceled) {
			t.Fatalf("invocation context error = %v, want cancellation", observed)
		}
	case <-time.After(time.Second):
		t.Fatal("invocation did not observe cancellation")
	}
	select {
	case callErr := <-callDone:
		if !errors.Is(callErr, context.Canceled) {
			t.Fatalf("call error = %v, want context cancellation", callErr)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled call did not return")
	}
}

func TestServerBoundsConcurrentInvocations(t *testing.T) {
	const callCount = 4
	var (
		mutex     sync.Mutex
		active    int
		maxActive int
	)
	started := make(chan struct{}, callCount)
	release := make(chan struct{})
	server, err := newServer(app.Dependencies{}, Options{
		Concurrency:     2,
		RequestInterval: time.Nanosecond,
	}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		mutex.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mutex.Unlock()
		started <- struct{}{}
		<-release
		mutex.Lock()
		active--
		mutex.Unlock()
		return validPosterResult()
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)
	callErrors := make(chan error, callCount)
	for range callCount {
		go func() {
			_, callErr := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "posters_list",
				Arguments: map[string]any{},
			})
			callErrors <- callErr
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("two invocations did not start concurrently")
		}
	}
	select {
	case <-started:
		t.Fatal("more than two invocations started before a slot was released")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for range callCount {
		if callErr := <-callErrors; callErr != nil {
			t.Fatalf("call tool: %v", callErr)
		}
	}
	mutex.Lock()
	defer mutex.Unlock()
	if maxActive != 2 {
		t.Fatalf("maximum active invocations = %d, want 2", maxActive)
	}
}

func TestServerRejectsDuplicateRequestIDsWithoutDoubleInvocation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	duplicated := make(chan struct{})
	var (
		invocations atomic.Int32
		startOnce   sync.Once
	)
	server, err := newServer(
		app.Dependencies{},
		Options{RequestInterval: time.Nanosecond},
		func(
			context.Context,
			string,
			map[string]any,
			app.Dependencies,
			...app.MCPExecutionOptions,
		) app.Result {
			invocations.Add(1)
			startOnce.Do(func() { close(started) })
			<-release
			return validPosterResult()
		},
	)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "partiful-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &duplicateRequestTransport{
		delegate:   clientTransport,
		started:    started,
		duplicated: duplicated,
	}, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	type callResult struct {
		result *mcp.CallToolResult
		err    error
	}
	callDone := make(chan callResult, 1)
	go func() {
		result, callErr := session.CallTool(ctx, &mcp.CallToolParams{
			Name:      "posters_list",
			Arguments: map[string]any{},
		})
		callDone <- callResult{result: result, err: callErr}
	}()

	select {
	case <-duplicated:
		close(release)
	case <-ctx.Done():
		t.Fatal("duplicate request was not written while the original was in flight")
	}
	call := <-callDone
	if call.err != nil || call.result == nil || call.result.IsError {
		t.Fatalf("original request result = %#v, error = %v; want success", call.result, call.err)
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("invocations = %d, want one for duplicate request ID", got)
	}
}

func TestServerAcceptsCanonicalInputsForEveryTool(t *testing.T) {
	validInputs := map[string]map[string]any{
		"blasts_send": {
			"eventId": "event-example", "audience": "all-guests", "message": "Hello", "dryRun": true,
		},
		"cohosts_invite": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"cohosts_link_create": {
			"eventId": "event-example", "dryRun": true,
		},
		"cohosts_link_revoke": {
			"eventId": "event-example", "dryRun": true,
		},
		"cohosts_remove": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"cohosts_revoke_invite": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"contacts_list": {},
		"events_cancel": {
			"eventId": "event-example", "message": "", "notifyGuests": true, "dryRun": true,
		},
		"events_create": {
			"title": "Example", "start": "2026-09-12T19:00:00Z", "timezone": "UTC",
			"description": nil, "guestLimit": 75,
			"links":  []any{map[string]any{"label": "Tickets", "url": "https://example.test/tickets"}},
			"dryRun": true,
		},
		"events_get":  {"eventId": "event-example"},
		"events_list": {"when": "upcoming"},
		"events_update": {
			"eventId": "event-example", "description": nil, "guestLimit": 75,
			"links":  []any{map[string]any{"label": "Tickets", "url": "https://example.test/tickets"}},
			"dryRun": true,
		},
		"guests_invite": {
			"eventId": "event-example", "contact": "Example Contact", "dryRun": true,
		},
		"guests_list":    {"eventId": "event-example"},
		"posters_list":   {},
		"posters_search": {"query": "party"},
		"rsvp_get":       {"eventId": "event-example"},
		"rsvp_set": {
			"eventId": "event-example", "status": "going", "displayName": "Example Attendee",
			"partySize": 2, "plusOnes": []any{"Guest One"}, "message": nil, "timezone": "UTC",
			"questionnaireResponse": map[string]any{
				"questionnaireVersion": 0,
				"answers":              map[string]any{"question-example": "Answer"},
			},
			"dryRun": true,
		},
	}
	called := map[string]bool{}
	server, err := newServer(app.Dependencies{}, Options{RequestInterval: time.Nanosecond}, func(
		_ context.Context,
		name string,
		_ map[string]any,
		_ app.Dependencies,
		_ ...app.MCPExecutionOptions,
	) app.Result {
		called[name] = true
		return app.Result{
			Stdout:   `{"ok":false,"error":{"type":"auth.required","code":"AUTHENTICATION_REQUIRED","message":"Authentication is required. Log in and try again.","retryable":false,"details":{}},"meta":{"command":"test","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test"}}` + "\n",
			ExitCode: 3,
		}
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	session := connectInMemory(t, server)

	for name, arguments := range validInputs {
		t.Run(name, func(t *testing.T) {
			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      name,
				Arguments: arguments,
			})
			if err != nil {
				t.Fatalf("schema-valid call rejected: %v", err)
			}
			if result == nil || !result.IsError || !called[name] {
				t.Fatalf("result = %#v, called = %t; want invocation", result, called[name])
			}
		})
	}
	if len(called) != len(validInputs) {
		t.Fatalf("invoked tools = %v, want all %d", called, len(validInputs))
	}
}

func connectInMemory(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	if _, err := server.Connect(context.Background(), serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "partiful-test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func validPosterResult() app.Result {
	return app.Result{Stdout: `{"ok":true,"data":{"items":[]},"meta":{"command":"posters.list","cliVersion":"test","productContractRevision":"test","remoteContractRevision":"test","warnings":[]}}` + "\n"}
}

func assertToolErrorCode(t *testing.T, result *mcp.CallToolResult, code string) {
	t.Helper()
	if result == nil || !result.IsError || len(result.Content) != 1 {
		t.Fatalf("result = %#v, want tool error", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(text.Text, `"code":"`+code+`"`) {
		t.Fatalf("result = %#v, want error code %s", result, code)
	}
	structured, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured content: %v", err)
	}
	var textEnvelope, structuredEnvelope any
	if err := json.Unmarshal([]byte(text.Text), &textEnvelope); err != nil {
		t.Fatalf("decode text content: %v", err)
	}
	if err := json.Unmarshal(structured, &structuredEnvelope); err != nil {
		t.Fatalf("decode structured content: %v", err)
	}
	if !reflect.DeepEqual(textEnvelope, structuredEnvelope) {
		t.Fatalf("text = %s, structured = %s", text.Text, structured)
	}
}

func assertSameJSON(t *testing.T, actual any, expected json.RawMessage) {
	t.Helper()
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		t.Fatalf("marshal actual JSON: %v", err)
	}
	var actualValue, expectedValue any
	if err := json.Unmarshal(actualJSON, &actualValue); err != nil {
		t.Fatalf("decode actual JSON: %v", err)
	}
	if err := json.Unmarshal(expected, &expectedValue); err != nil {
		t.Fatalf("decode expected JSON: %v", err)
	}
	if !reflect.DeepEqual(actualValue, expectedValue) {
		t.Fatalf("actual JSON = %s, want %s", actualJSON, expected)
	}
}

func assertToolTextWithinLimit(t *testing.T, result *mcp.CallToolResult, limit int) {
	t.Helper()
	if result == nil || len(result.Content) != 1 {
		t.Fatalf("result = %#v, want one content item", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content type = %T, want text", result.Content[0])
	}
	if len(text.Text) > limit {
		t.Fatalf("tool text length = %d, want at most %d", len(text.Text), limit)
	}
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

type httpClientFunc func(*http.Request) (*http.Response, error)

func (function httpClientFunc) Do(request *http.Request) (*http.Response, error) {
	return function(request)
}

type duplicateRequestTransport struct {
	delegate   mcp.Transport
	started    <-chan struct{}
	duplicated chan<- struct{}
}

func (transport *duplicateRequestTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := transport.delegate.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &duplicateRequestConnection{
		Connection: connection,
		started:    transport.started,
		duplicated: transport.duplicated,
	}, nil
}

type duplicateRequestConnection struct {
	mcp.Connection
	started    <-chan struct{}
	duplicated chan<- struct{}
	once       sync.Once
}

func (connection *duplicateRequestConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	request, ok := message.(*jsonrpc.Request)
	if !ok || request.Method != "tools/call" {
		return connection.Connection.Write(ctx, message)
	}
	duplicateThisRequest := false
	connection.once.Do(func() {
		duplicateThisRequest = true
	})
	if !duplicateThisRequest {
		return connection.Connection.Write(ctx, message)
	}
	if err := connection.Connection.Write(ctx, message); err != nil {
		return err
	}
	select {
	case <-connection.started:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err := connection.Connection.Write(ctx, message); err != nil {
		return err
	}
	close(connection.duplicated)
	return nil
}
