package app_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestExecuteConventionalHelp(t *testing.T) {
	noNetwork := scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("help must not make network calls")
		return nil, nil
	}}

	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{
			name: "root long help",
			argv: []string{"--help"},
			want: []string{"Usage: partiful <command>", "Commands:", "events", "Run 'partiful help <command path>'"},
		},
		{
			name: "root short help",
			argv: []string{"-h"},
			want: []string{"Usage: partiful <command>", "Commands:"},
		},
		{
			name: "group short help",
			argv: []string{"events", "-h"},
			want: []string{"Usage: partiful events <command>", "Commands:", "create", "update"},
		},
		{
			name: "leaf long help",
			argv: []string{"events", "create", "--help"},
			want: []string{
				"Usage: partiful events create", "Purpose:", "Flags:", "--plan <value>", "--apply",
				"Required fields:", "title", "start", "timezone", "Examples:", "Exit behavior:",
				"Mutation safety:", "repeat the original payload", "partiful events create --title <value>",
			},
		},
		{
			name: "help command leaf path",
			argv: []string{"help", "events", "create"},
			want: []string{"Usage: partiful events create", "--plan <value>", "Mutation safety:"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := app.Execute(context.Background(), app.Request{Argv: test.argv}, app.Dependencies{HTTP: noNetwork})
			if result.ExitCode != 0 || result.Stderr != "" {
				t.Fatalf("result = %#v, want successful local help", result)
			}
			for _, fragment := range test.want {
				if !strings.Contains(result.Stdout, fragment) {
					t.Fatalf("help output = %q, want %q", result.Stdout, fragment)
				}
			}
		})
	}
}

func TestExecuteHelpRetainsUnknownFlagFailure(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{"events", "create", "--unknown"},
	}, app.Dependencies{})
	if result.ExitCode != 2 || !strings.Contains(result.Stdout, `"code":"FLAG_UNKNOWN"`) {
		t.Fatalf("result = %#v, want FLAG_UNKNOWN", result)
	}
}

func TestExecuteHelpCoversEverySchemaCommandWithoutDependencies(t *testing.T) {
	catalog := app.Execute(context.Background(), app.Request{Argv: []string{"schema"}}, app.Dependencies{})
	var envelope struct {
		Data struct {
			Commands []string `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(catalog.Stdout), &envelope); err != nil {
		t.Fatalf("decode schema catalog: %v", err)
	}

	noNetwork := scriptedHTTP{do: func(*http.Request) (*http.Response, error) {
		t.Fatal("help must not make network calls")
		return nil, nil
	}}
	for _, command := range envelope.Data.Commands {
		t.Run(command, func(t *testing.T) {
			path := strings.Fields(strings.ReplaceAll(command, ".", " "))
			if command == "version" {
				path = []string{"--version"}
			}
			argv := append([]string{"help"}, path...)
			result := app.Execute(context.Background(), app.Request{Argv: argv}, app.Dependencies{HTTP: noNetwork})
			if result.ExitCode != 0 || !strings.Contains(result.Stdout, "Purpose:") {
				t.Fatalf("help %s = %#v, want local leaf help", command, result)
			}
		})
	}
}
