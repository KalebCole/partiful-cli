package mcpserver

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestParseOptionsRejectsUnknownAndSupportsCommaSeparatedFilters(t *testing.T) {
	options, err := ParseOptions([]string{
		"--read-only",
		"--allow-tool", "events_*,posters_list",
		"--timeout", "2s",
		"--max-concurrency", "3",
		"--max-output-bytes", "4096",
		"--max-items", "17",
		"--request-interval", "25ms",
	})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if !options.ReadOnly ||
		!reflect.DeepEqual(options.AllowTools, []string{"events_*", "posters_list"}) ||
		options.CallTimeout != 2*time.Second ||
		options.Concurrency != 3 ||
		options.MaxBytes != 4096 ||
		options.MaxItems != 17 ||
		options.RequestInterval != 25*time.Millisecond {
		t.Fatalf("options = %#v", options)
	}
	if _, err := ParseOptions([]string{"--allow-write"}); err == nil {
		t.Fatal("unknown option accepted")
	}
}

func TestParseOptionsRejectsUnsafeLimits(t *testing.T) {
	for _, argv := range [][]string{
		{"--timeout", "0s"},
		{"--max-concurrency", "0"},
		{"--max-output-bytes", "511"},
		{"--max-items", "0"},
		{"--request-interval", "-1ms"},
	} {
		if _, err := ParseOptions(argv); err == nil {
			t.Fatalf("options %v accepted", argv)
		}
	}
}

func TestServerRejectsOutputLimitTooSmallForStandardErrors(t *testing.T) {
	if _, err := newServer(app.Dependencies{}, Options{MaxBytes: 511}, func(
		context.Context,
		string,
		map[string]any,
		app.Dependencies,
		...app.MCPExecutionOptions,
	) app.Result {
		return validPosterResult()
	}); err == nil {
		t.Fatal("server accepted an output limit too small for standard MCP errors")
	}
}

func TestEnabledToolsDefaultAndNarrowing(t *testing.T) {
	all, err := EnabledTools(Options{})
	if err != nil {
		t.Fatalf("default enabled tools: %v", err)
	}
	if len(all) != len(app.MCPDefinitions()) {
		t.Fatalf("default = %d, want %d", len(all), len(app.MCPDefinitions()))
	}
	foundDefaultWrite := false
	for _, tool := range all {
		if tool.Name == "events_create" && !tool.ReadOnly {
			foundDefaultWrite = true
		}
	}
	if !foundDefaultWrite {
		t.Fatal("default tool set does not expose write operations")
	}
	readOnly, err := EnabledTools(Options{ReadOnly: true})
	if err != nil {
		t.Fatalf("read-only enabled tools: %v", err)
	}
	for _, tool := range readOnly {
		if !tool.ReadOnly {
			t.Fatalf("write tool %q included in read-only server", tool.Name)
		}
	}
	selected, err := EnabledTools(Options{AllowTools: []string{"events_*", "posters_list"}})
	if err != nil {
		t.Fatalf("filtered enabled tools: %v", err)
	}
	if len(selected) != 6 {
		t.Fatalf("filtered tools = %d, want 6", len(selected))
	}
	if _, err := EnabledTools(Options{AllowTools: []string{"no_such_tool"}}); err == nil {
		t.Fatal("unknown allow tool accepted")
	}
	if _, err := EnabledTools(Options{
		ReadOnly:   true,
		AllowTools: []string{"events_create"},
	}); err == nil {
		t.Fatal("allow-tool widened a read-only server")
	}
}
