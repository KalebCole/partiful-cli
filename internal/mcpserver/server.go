package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Options struct {
	ReadOnly    bool
	AllowTools  []string
	CallTimeout time.Duration
	Concurrency int
	MaxBytes    int
}

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
		default:
			return Options{}, fmt.Errorf("unknown mcp option %q", argv[index])
		}
	}
	if _, err := EnabledTools(options); err != nil {
		return Options{}, err
	}
	return options, nil
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
	server := mcp.NewServer(&mcp.Implementation{Name: "partiful-cli", Title: "Partiful CLI", Version: app.Version}, nil)
	semaphore := make(chan struct{}, options.Concurrency)
	for _, definition := range tools {
		definition := definition
		server.AddTool(&mcp.Tool{
			Name: definition.Name, Description: definition.Description,
			InputSchema: definition.InputSchema, OutputSchema: definition.OutputSchema,
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: definition.ReadOnly, DestructiveHint: boolPtr(definition.Destructive)},
		}, func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				return toolError(`{"ok":false,"error":{"type":"remote.unavailable","code":"MCP_CALL_CANCELLED","message":"The MCP call was cancelled.","retryable":true,"details":{}}}`), nil
			}
			callContext, cancel := context.WithTimeout(ctx, options.CallTimeout)
			defer cancel()
			arguments := map[string]any{}
			if len(request.Params.Arguments) > 0 && json.Unmarshal(request.Params.Arguments, &arguments) != nil {
				return toolError(`{"ok":false,"error":{"type":"input.invalid","code":"MCP_ARGUMENTS_INVALID","message":"Tool arguments must be a JSON object.","retryable":false,"details":{}}}`), nil
			}
			result := app.ExecuteMCP(callContext, definition.Name, arguments, dependencies)
			body := strings.TrimSpace(result.Stdout)
			if len(body) > options.MaxBytes {
				body = `{"ok":false,"error":{"type":"input.invalid","code":"MCP_OUTPUT_LIMIT","message":"Tool output exceeds the configured limit; reduce page size.","retryable":false,"details":{}}}`
			}
			var structured any
			if err := json.Unmarshal([]byte(body), &structured); err != nil {
				return nil, fmt.Errorf("application returned invalid JSON: %w", err)
			}
			if result.ExitCode != 0 {
				return toolError(body), nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: body}}, StructuredContent: structured}, nil
		})
	}
	return server, nil
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
