package app_test

import (
	encodingjson "encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

type openAPIDocument struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}

func TestRemoteContractRevisionMatchesOpenAPI(t *testing.T) {
	root := repoRoot(t)
	var openapi openAPIDocument
	if err := encodingjson.Unmarshal([]byte(readRepoFile(t, root, "spec/partiful.openapi.json")), &openapi); err != nil {
		t.Fatalf("decode spec/partiful.openapi.json: %v", err)
	}
	if openapi.Info.Version != app.RemoteContractRevision {
		t.Fatalf("openapi version = %q, want %q", openapi.Info.Version, app.RemoteContractRevision)
	}
}
