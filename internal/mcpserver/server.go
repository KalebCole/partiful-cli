package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/remote"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	ReadOnly        bool
	AllowTools      []string
	CallTimeout     time.Duration
	Concurrency     int
	MaxBytes        int
	MaxItems        int
	RequestInterval time.Duration
}

type toolInvoker func(
	context.Context,
	string,
	map[string]any,
	app.Dependencies,
	...app.MCPExecutionOptions,
) app.Result

const (
	mcpArgumentsInvalidEnvelope = `{"ok":false,"error":{"type":"input.invalid","code":"MCP_ARGUMENTS_INVALID","message":"Tool arguments do not match the published input schema.","retryable":false,"details":{}}}`
	mcpOutputInvalidEnvelope    = `{"ok":false,"error":{"type":"internal.failure","code":"MCP_OUTPUT_INVALID","message":"Tool output did not match the published output schema.","retryable":false,"details":{}}}`
)

func ParseOptions(argv []string) (Options, error) {
	options := Options{}
	for index := 0; index < len(argv); index++ {
		switch argv[index] {
		case "--read-only":
			if options.ReadOnly {
				return Options{}, fmt.Errorf("--read-only cannot be repeated")
			}
			options.ReadOnly = true
		case "--allow-tool":
			if index+1 >= len(argv) {
				return Options{}, fmt.Errorf("--allow-tool requires a selector")
			}
			index++
			for _, selector := range strings.Split(argv[index], ",") {
				options.AllowTools = append(options.AllowTools, selector)
			}
		case "--list-tools":
			// The command entrypoint consumes this display-only flag.
		case "--timeout":
			value, valueError := optionValue(argv, &index, "--timeout")
			if valueError != nil {
				return Options{}, valueError
			}
			options.CallTimeout, valueError = time.ParseDuration(value)
			if valueError != nil || options.CallTimeout <= 0 {
				return Options{}, fmt.Errorf("--timeout must be a positive duration")
			}
		case "--max-concurrency":
			value, valueError := positiveIntegerOption(argv, &index, "--max-concurrency")
			if valueError != nil {
				return Options{}, valueError
			}
			options.Concurrency = value
		case "--max-output-bytes":
			value, valueError := positiveIntegerOption(argv, &index, "--max-output-bytes")
			if valueError != nil {
				return Options{}, valueError
			}
			if value < 256 {
				return Options{}, fmt.Errorf("--max-output-bytes must be at least 256")
			}
			options.MaxBytes = value
		case "--max-items":
			value, valueError := positiveIntegerOption(argv, &index, "--max-items")
			if valueError != nil {
				return Options{}, valueError
			}
			if value > 1000 {
				return Options{}, fmt.Errorf("--max-items must not exceed 1000")
			}
			options.MaxItems = value
		case "--request-interval":
			value, valueError := optionValue(argv, &index, "--request-interval")
			if valueError != nil {
				return Options{}, valueError
			}
			options.RequestInterval, valueError = time.ParseDuration(value)
			if valueError != nil || options.RequestInterval <= 0 {
				return Options{}, fmt.Errorf("--request-interval must be a positive duration")
			}
		default:
			return Options{}, fmt.Errorf("unknown mcp option %q", argv[index])
		}
	}
	if _, err := EnabledTools(options); err != nil {
		return Options{}, err
	}
	return options, nil
}

func optionValue(argv []string, index *int, name string) (string, error) {
	if *index+1 >= len(argv) {
		return "", fmt.Errorf("%s requires a value", name)
	}
	(*index)++
	return argv[*index], nil
}

func positiveIntegerOption(argv []string, index *int, name string) (int, error) {
	raw, err := optionValue(argv, index, name)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

func EnabledTools(options Options) ([]app.MCPDefinition, error) {
	all := app.MCPDefinitions()
	available := make([]app.MCPDefinition, 0, len(all))
	for _, tool := range all {
		if options.ReadOnly && !tool.ReadOnly {
			continue
		}
		available = append(available, tool)
	}
	if len(options.AllowTools) == 0 {
		return available, nil
	}
	selected := make([]app.MCPDefinition, 0, len(available))
	matched := make(map[string]bool)
	for _, selector := range options.AllowTools {
		selector = strings.TrimSpace(selector)
		if selector == "" {
			return nil, fmt.Errorf("--allow-tool cannot be empty")
		}
		for _, tool := range available {
			if allowMatch(selector, tool.Name) {
				matched[selector] = true
			}
		}
		if !matched[selector] {
			return nil, fmt.Errorf("--allow-tool %q does not select an enabled tool", selector)
		}
	}
	for _, tool := range available {
		for _, selector := range options.AllowTools {
			if allowMatch(strings.TrimSpace(selector), tool.Name) {
				selected = append(selected, tool)
				break
			}
		}
	}
	return selected, nil
}

func allowMatch(selector, name string) bool {
	if strings.Count(selector, "*") > 1 || (strings.Contains(selector, "*") && !strings.HasSuffix(selector, "*")) {
		return false
	}
	if strings.HasSuffix(selector, "*") {
		return strings.HasPrefix(name, strings.TrimSuffix(selector, "*"))
	}
	return selector == name
}

func New(dependencies app.Dependencies, options Options) (*mcp.Server, error) {
	return newServer(dependencies, options, app.ExecuteMCP)
}

func newServer(
	dependencies app.Dependencies,
	options Options,
	invoke toolInvoker,
) (*mcp.Server, error) {
	return newServerWithSDKOptions(dependencies, options, invoke, nil)
}

func newServerWithSDKOptions(
	dependencies app.Dependencies,
	options Options,
	invoke toolInvoker,
	sdkOptions *mcp.ServerOptions,
) (*mcp.Server, error) {
	tools, err := EnabledTools(options)
	if err != nil {
		return nil, err
	}
	if options.CallTimeout <= 0 {
		options.CallTimeout = 30 * time.Second
	}
	if options.Concurrency <= 0 {
		options.Concurrency = 4
	}
	if options.MaxBytes <= 0 {
		options.MaxBytes = 256 << 10
	}
	if options.MaxItems <= 0 {
		options.MaxItems = 100
	}
	if options.RequestInterval < 0 {
		return nil, fmt.Errorf("request interval must not be negative")
	}
	if options.RequestInterval == 0 {
		options.RequestInterval = 100 * time.Millisecond
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "partiful-cli", Title: "Partiful CLI", Version: app.Version},
		sdkOptions,
	)
	semaphore := make(chan struct{}, options.Concurrency)
	pacer := requestPacer{interval: options.RequestInterval}
	if dependencies.HTTP != nil {
		dependencies.HTTP = pacedHTTPClient{delegate: dependencies.HTTP, pacer: &pacer}
	}
	for _, definition := range tools {
		definition := definition
		inputSchema, err := resolveToolSchema(definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("resolve input schema for %q: %w", definition.Name, err)
		}
		outputSchema, err := resolveToolSchema(definition.OutputSchema)
		if err != nil {
			return nil, fmt.Errorf("resolve output schema for %q: %w", definition.Name, err)
		}
		server.AddTool(&mcp.Tool{
			Name: definition.Name, Description: definition.Description,
			InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: definition.ReadOnly, DestructiveHint: boolPtr(definition.Destructive)},
		}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var rawArguments json.RawMessage
			if request != nil && request.Params != nil {
				rawArguments = request.Params.Arguments
			}
			arguments, valid := validateToolArguments(rawArguments, inputSchema)
			if !valid {
				return toolError(mcpArgumentsInvalidEnvelope), nil
			}
			callContext, cancel := context.WithTimeout(ctx, options.CallTimeout)
			defer cancel()
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-callContext.Done():
				return toolError(callContextError(callContext)), nil
			}
			result := invoke(
				callContext,
				definition.Name,
				arguments,
				dependencies,
				app.MCPExecutionOptions{MaxItems: options.MaxItems},
			)
			mutationMayHaveDispatched := !definition.ReadOnly && !dryRun(arguments)
			if callContext.Err() != nil && !mutationMayHaveDispatched {
				return toolError(callContextError(callContext)), nil
			}
			body := strings.TrimSpace(result.Stdout)
			outputLimited := len(body) > options.MaxBytes
			if outputLimited {
				if mutationMayHaveDispatched {
					body = `{"ok":false,"error":{"type":"remote.unavailable","code":"MCP_MUTATION_OUTCOME_UNCERTAIN","message":"Mutation result exceeded the output limit; inspect remote state before another attempt.","retryable":false,"details":{}}}`
				} else {
					body = `{"ok":false,"error":{"type":"input.invalid","code":"MCP_OUTPUT_LIMIT","message":"Tool output exceeds the configured limit; reduce page size.","retryable":false,"details":{}}}`
				}
			}
			if !json.Valid([]byte(body)) {
				return toolError(mcpOutputInvalidEnvelope), nil
			}
			if result.ExitCode != 0 || outputLimited {
				return toolError(body), nil
			}
			if !validateToolOutput(json.RawMessage(body), outputSchema) {
				return toolError(mcpOutputInvalidEnvelope), nil
			}
			return &mcp.CallToolResult{
				Content:           []mcp.Content{&mcp.TextContent{Text: body}},
				StructuredContent: json.RawMessage(body),
			}, nil
		})
	}
	return server, nil
}

func resolveToolSchema(raw json.RawMessage) (*jsonschema.Resolved, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		return nil, err
	}
	return schema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
}

func validateToolArguments(
	raw json.RawMessage,
	schema *jsonschema.Resolved,
) (map[string]any, bool) {
	var value any = map[string]any{}
	if len(raw) > 0 {
		var valid bool
		value, valid = decodeToolJSON(raw)
		if !valid {
			return nil, false
		}
	}
	arguments, valid := value.(map[string]any)
	if arguments == nil {
		return nil, false
	}
	if err := schema.ApplyDefaults(&value); err != nil {
		return nil, false
	}
	if err := schema.Validate(value); err != nil {
		return nil, false
	}
	arguments, valid = value.(map[string]any)
	return arguments, valid
}

func validateToolOutput(
	raw json.RawMessage,
	schema *jsonschema.Resolved,
) bool {
	output, valid := decodeToolJSON(raw)
	if !valid {
		return false
	}
	if err := schema.Validate(output); err != nil {
		return false
	}
	// Preserve the application bytes after validation so large JSON integers
	// and the already-enforced output length cannot change in transit.
	return true
}

func decodeToolJSON(raw json.RawMessage) (any, bool) {
	if !json.Valid(raw) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return normalizeToolJSONNumbers(value)
}

func normalizeToolJSONNumbers(value any) (any, bool) {
	switch value := value.(type) {
	case json.Number:
		return normalizeToolJSONNumber(value)
	case map[string]any:
		for key, item := range value {
			normalized, valid := normalizeToolJSONNumbers(item)
			if !valid {
				return nil, false
			}
			value[key] = normalized
		}
	case []any:
		for index, item := range value {
			normalized, valid := normalizeToolJSONNumbers(item)
			if !valid {
				return nil, false
			}
			value[index] = normalized
		}
	}
	return value, true
}

func normalizeToolJSONNumber(number json.Number) (any, bool) {
	exact, valid := new(big.Rat).SetString(number.String())
	if !valid {
		return nil, false
	}
	if exact.IsInt() {
		if !exact.Num().IsInt64() {
			return nil, false
		}
		return exact.Num().Int64(), true
	}
	approximate, err := number.Float64()
	if err != nil || math.IsInf(approximate, 0) || math.IsNaN(approximate) {
		return nil, false
	}
	if _, fraction := math.Modf(approximate); fraction != 0 {
		return approximate, true
	}
	rounded := new(big.Rat).SetFloat64(approximate)
	switch exact.Cmp(rounded) {
	case -1:
		approximate = math.Nextafter(approximate, math.Inf(-1))
	case 1:
		approximate = math.Nextafter(approximate, math.Inf(1))
	}
	if math.IsInf(approximate, 0) {
		return nil, false
	}
	if _, fraction := math.Modf(approximate); fraction == 0 {
		return nil, false
	}
	return approximate, true
}

type requestPacer struct {
	mutex    sync.Mutex
	interval time.Duration
	next     time.Time
}

type pacedHTTPClient struct {
	delegate remote.HTTPClient
	pacer    *requestPacer
}

func (client pacedHTTPClient) Do(request *http.Request) (*http.Response, error) {
	if err := client.pacer.Wait(request.Context()); err != nil {
		return nil, err
	}
	return client.delegate.Do(request)
}

func (pacer *requestPacer) Wait(ctx context.Context) error {
	pacer.mutex.Lock()
	now := time.Now()
	start := now
	if pacer.next.After(start) {
		start = pacer.next
	}
	pacer.next = start.Add(pacer.interval)
	pacer.mutex.Unlock()

	delay := time.Until(start)
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func callContextError(ctx context.Context) string {
	if ctx.Err() == context.DeadlineExceeded {
		return `{"ok":false,"error":{"type":"remote.unavailable","code":"MCP_CALL_TIMEOUT","message":"The MCP call exceeded the configured timeout.","retryable":true,"details":{}}}`
	}
	return `{"ok":false,"error":{"type":"remote.unavailable","code":"MCP_CALL_CANCELLED","message":"The MCP call was cancelled.","retryable":true,"details":{}}}`
}

func dryRun(arguments map[string]any) bool {
	value, _ := arguments["dryRun"].(bool)
	return value
}

func Run(ctx context.Context, dependencies app.Dependencies, options Options) error {
	server, err := New(dependencies, options)
	if err != nil {
		return err
	}
	return server.Run(ctx, &mcp.StdioTransport{})
}

func toolError(text string) *mcp.CallToolResult {
	var structured any
	_ = json.Unmarshal([]byte(text), &structured)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: text}}, StructuredContent: structured}
}

func boolPtr(value bool) *bool { return &value }
