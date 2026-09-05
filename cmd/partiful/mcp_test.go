package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPStdioWithOfficialSDK(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "partiful")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	home := t.TempDir()
	command := exec.Command(binary, "mcp")
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
	if len(names) != 18 || !slices.Contains(names, "events_create") || !slices.Contains(names, "blasts_send") || slices.Contains(names, "auth_login") {
		t.Fatalf("tools = %v", names)
	}
	read, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "events_list", Arguments: json.RawMessage(`{"when":"upcoming"}`)})
	if err != nil || !read.IsError || read.StructuredContent == nil {
		t.Fatalf("protected read = %#v, %v; want structured auth tool error", read, err)
	}
	preview, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "events_create", Arguments: json.RawMessage(`{"title":"Example","start":"2026-09-12T19:00:00Z","timezone":"UTC","dryRun":true}`)})
	if err != nil || preview.IsError || preview.StructuredContent == nil {
		t.Fatalf("dry-run mutation = %#v, %v; want structured preview without terminal confirmation", preview, err)
	}
}
