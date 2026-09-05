package app_test

import (
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestMCPDefinitionsExposeOnlyProductOperations(t *testing.T) {
	definitions := app.MCPDefinitions()
	if len(definitions) != 18 {
		t.Fatalf("definition count = %d, want 18", len(definitions))
	}
	seen := map[string]bool{}
	for _, definition := range definitions {
		if seen[definition.Name] {
			t.Fatalf("duplicate MCP tool %q", definition.Name)
		}
		seen[definition.Name] = true
		if definition.Command == "auth.login" || definition.Command == "auth.logout" || definition.Command == "schema" || definition.Command == "doctor" || definition.Command == "version" {
			t.Fatalf("excluded command exposed: %q", definition.Command)
		}
		if len(definition.InputSchema) == 0 || len(definition.OutputSchema) == 0 {
			t.Fatalf("tool %q has empty schemas", definition.Name)
		}
	}
	for _, name := range []string{"events_create", "events_cancel", "blasts_send", "posters_list", "rsvp_set"} {
		if !seen[name] {
			t.Fatalf("missing MCP tool %q", name)
		}
	}
}
