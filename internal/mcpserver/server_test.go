package mcpserver

import (
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestParseOptionsRejectsUnknownAndSupportsCommaSeparatedFilters(t *testing.T) {
	options, err := ParseOptions([]string{"--read-only", "--allow-tool", "events_*,posters_list"})
	if err != nil {
		t.Fatalf("parse options: %v", err)
	}
	if !options.ReadOnly || len(options.AllowTools) != 2 {
		t.Fatalf("options = %#v", options)
	}
	if _, err := ParseOptions([]string{"--allow-write"}); err == nil {
		t.Fatal("unknown option accepted")
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
}
