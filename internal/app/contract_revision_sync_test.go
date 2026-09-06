package app_test

import (
	encodingjson "encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

type evidenceDocument struct {
	ContractRevision string                             `json:"contractRevision"`
	Status           string                             `json:"status"`
	Operations       map[string]encodingjson.RawMessage `json:"operations"`
	OperationClaims  map[string]map[string]string       `json:"operationClaims"`
}

type openAPIDocument struct {
	Info struct {
		Version string `json:"version"`
	} `json:"info"`
	Paths map[string]encodingjson.RawMessage `json:"paths"`
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

func TestContractRevisionsStayInSyncAcrossDocsAndSpecs(t *testing.T) {
	const exploreContractRevision = "2026-09-05.1"
	if app.ProductContractRevision != exploreContractRevision {
		t.Fatalf("product contract revision = %q, want %q", app.ProductContractRevision, exploreContractRevision)
	}
	if app.RemoteContractRevision != exploreContractRevision {
		t.Fatalf("remote contract revision = %q, want %q", app.RemoteContractRevision, exploreContractRevision)
	}

	root := repoRoot(t)
	productDoc := readRepoFile(t, root, "docs/CLI-PRODUCT-CONTRACT.md")
	if !strings.Contains(productDoc, "**Product contract revision:** `"+app.ProductContractRevision+"`") {
		t.Fatalf("product contract doc does not publish revision %q", app.ProductContractRevision)
	}
	if !strings.Contains(productDoc, "**Remote API contract revision:** `"+app.RemoteContractRevision+"`") {
		t.Fatalf("product contract doc does not publish remote revision %q", app.RemoteContractRevision)
	}

	remoteDoc := readRepoFile(t, root, "docs/REMOTE-API-CONTRACT.md")
	if !strings.Contains(remoteDoc, "owner-reviewed revision `"+app.RemoteContractRevision+"`") {
		t.Fatalf("remote contract doc does not publish revision %q", app.RemoteContractRevision)
	}
	if !strings.Contains(remoteDoc, "The Go CLI ships remote contract revision `"+app.RemoteContractRevision+"`.") {
		t.Fatalf("remote contract doc does not bind the Go CLI to revision %q", app.RemoteContractRevision)
	}

	var evidence evidenceDocument
	if err := encodingjson.Unmarshal([]byte(readRepoFile(t, root, "spec/partiful.api-evidence.json")), &evidence); err != nil {
		t.Fatalf("decode spec/partiful.api-evidence.json: %v", err)
	}
	if evidence.ContractRevision != app.RemoteContractRevision {
		t.Fatalf("evidence contract revision = %q, want %q", evidence.ContractRevision, app.RemoteContractRevision)
	}
	if evidence.Status != "owner-reviewed" {
		t.Fatalf("evidence status = %q, want owner-reviewed", evidence.Status)
	}
	for _, operation := range []string{"getDiscoverFeedV2", "getDiscoverSectionsV2", "getDiscoverEventItemDecorators"} {
		if _, ok := evidence.Operations[operation]; !ok {
			t.Fatalf("evidence does not declare operation %q", operation)
		}
		claims, ok := evidence.OperationClaims[operation]
		if !ok {
			t.Fatalf("evidence does not declare claims for operation %q", operation)
		}
		for _, claim := range []string{"request", "response", "status"} {
			if claims[claim] == "" {
				t.Fatalf("evidence operation %q does not declare %s claim", operation, claim)
			}
		}
	}

	var openapi openAPIDocument
	if err := encodingjson.Unmarshal([]byte(readRepoFile(t, root, "spec/partiful.openapi.json")), &openapi); err != nil {
		t.Fatalf("decode spec/partiful.openapi.json: %v", err)
	}
	if openapi.Info.Version != app.RemoteContractRevision {
		t.Fatalf("openapi version = %q, want %q", openapi.Info.Version, app.RemoteContractRevision)
	}
	for _, path := range []string{"/getDiscoverFeedV2", "/getDiscoverSectionsV2", "/getDiscoverEventItemDecorators"} {
		if _, ok := openapi.Paths[path]; !ok {
			t.Fatalf("openapi does not declare path %q", path)
		}
	}
}
