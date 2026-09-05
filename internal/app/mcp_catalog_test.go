package app_test

import (
	"bytes"
	"reflect"
	"slices"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestMCPDefinitionsExposeOnlyProductOperations(t *testing.T) {
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
	definitions := app.MCPDefinitions()
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
		if len(definition.InputSchema) == 0 || len(definition.OutputSchema) == 0 {
			t.Fatalf("tool %q has empty schemas", definition.Name)
		}
		for _, forbidden := range [][]byte{[]byte(`"force"`), []byte(`"noInput"`), []byte(`"no-input"`)} {
			if bytes.Contains(definition.InputSchema, forbidden) {
				t.Fatalf("tool %q exposes CLI-only field %s", definition.Name, forbidden)
			}
		}
	}
	if !reflect.DeepEqual(names, wantNames) {
		t.Fatalf("tool names = %v, want exact curated set %v", names, wantNames)
	}
	for _, excluded := range []string{
		"auth_login",
		"auth_logout",
		"auth_status",
		"doctor",
		"schema",
		"version",
		"help",
		"mcp",
	} {
		if slices.Contains(names, excluded) {
			t.Fatalf("excluded tool exposed: %q", excluded)
		}
	}
}
