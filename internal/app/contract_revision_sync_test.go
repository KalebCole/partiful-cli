package app_test

import (
	encodingjson "encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

type evidenceClaim struct {
	Classification string `json:"classification"`
	Citation       string `json:"citation"`
}

type evidenceDocument struct {
	ContractRevision   string                             `json:"contractRevision"`
	Status             string                             `json:"status"`
	Operations         map[string]encodingjson.RawMessage `json:"operations"`
	OperationClaims    map[string]map[string]string       `json:"operationClaims"`
	Claims             map[string]evidenceClaim           `json:"claims"`
	MaterialClaimCount int                                `json:"materialClaimCount"`
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

func escapeJSONPointerToken(token string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(token)
}

func collectSchemaClaimPointers(schema any, pointer string, pointers map[string]struct{}) {
	object, ok := schema.(map[string]any)
	if !ok {
		return
	}
	for key, value := range object {
		childPointer := pointer + "/" + escapeJSONPointerToken(key)
		switch typed := value.(type) {
		case map[string]any:
			if key == "properties" {
				for property, propertySchema := range typed {
					propertyPointer := childPointer + "/" + escapeJSONPointerToken(property)
					pointers[propertyPointer] = struct{}{}
					collectSchemaClaimPointers(propertySchema, propertyPointer, pointers)
				}
				continue
			}
			collectSchemaClaimPointers(typed, childPointer, pointers)
		case []any:
			if key == "const" {
				pointers[childPointer] = struct{}{}
				continue
			}
			for index := range typed {
				pointers[childPointer+"/"+strconv.Itoa(index)] = struct{}{}
			}
		default:
			pointers[childPointer] = struct{}{}
		}
	}
}

func valueAt(document map[string]any, pointer string) (any, bool) {
	value := any(document)
	for _, rawToken := range strings.Split(strings.TrimPrefix(pointer, "#/"), "/") {
		token := strings.NewReplacer("~1", "/", "~0", "~").Replace(rawToken)
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		var found bool
		value, found = object[token]
		if !found {
			return nil, false
		}
	}
	return value, true
}

func schemaAt(t *testing.T, document map[string]any, pointer string) any {
	t.Helper()
	value, ok := valueAt(document, pointer)
	if !ok {
		t.Fatalf("%s is absent or has a non-object parent", pointer)
	}
	return value
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

	var rawOpenAPI map[string]any
	if err := encodingjson.Unmarshal([]byte(readRepoFile(t, root, "spec/partiful.openapi.json")), &rawOpenAPI); err != nil {
		t.Fatalf("decode raw spec/partiful.openapi.json: %v", err)
	}
	for pointer, want := range map[string][]string{
		"#/paths/~1getDiscoverFeedV2/post/requestBody/content/application~1json/schema/properties/data/properties/params/properties/allowedFeedPresentationStyles/const":        {"rows"},
		"#/paths/~1getDiscoverSectionsV2/post/requestBody/content/application~1json/schema/properties/data/properties/params/properties/allowedSectionPresentationStyles/const": {"carousel-small", "carousel-medium", "carousel-large"},
	} {
		actual, found := valueAt(rawOpenAPI, pointer)
		got, ok := actual.([]any)
		if !found || !ok || len(got) != len(want) {
			t.Errorf("%s = %#v, want exact array %#v", pointer, actual, want)
			continue
		}
		for index, value := range want {
			if got[index] != value {
				t.Errorf("%s[%d] = %#v, want %#v", pointer, index, got[index], value)
			}
		}
	}
	for pointer, want := range map[string]any{
		"#/paths/~1getDiscoverFeedV2/post/responses/200/content/application~1json/schema/properties/result/properties/data/properties/presentationStyle/type":    "string",
		"#/paths/~1getDiscoverFeedV2/post/responses/200/content/application~1json/schema/properties/result/properties/paging/properties/pageResultCount/type":    "integer",
		"#/paths/~1getDiscoverFeedV2/post/responses/200/content/application~1json/schema/properties/result/properties/paging/properties/pageResultCount/minimum": float64(0),
	} {
		if actual, found := valueAt(rawOpenAPI, pointer); !found || actual != want {
			t.Errorf("%s = %#v, want %#v", pointer, actual, want)
		}
	}

	for path, citation := range map[string]string{
		"/getDiscoverFeedV2":              "docs/research/2026-09-05-explore-v1-contract-evidence.md#getdiscoverfeedv2",
		"/getDiscoverSectionsV2":          "docs/research/2026-09-05-explore-v1-contract-evidence.md#getdiscoversectionsv2",
		"/getDiscoverEventItemDecorators": "docs/research/2026-09-05-explore-v1-contract-evidence.md#getdiscovereventitemdecorators",
	} {
		for _, schemaPointer := range []string{
			"#/paths/" + escapeJSONPointerToken(path) + "/post/requestBody/content/application~1json/schema",
			"#/paths/" + escapeJSONPointerToken(path) + "/post/responses/200/content/application~1json/schema",
		} {
			pointers := make(map[string]struct{})
			collectSchemaClaimPointers(schemaAt(t, rawOpenAPI, schemaPointer), schemaPointer, pointers)
			for claimPointer := range pointers {
				claim, ok := evidence.Claims[claimPointer]
				if !ok {
					t.Errorf("evidence lacks schema claim %s", claimPointer)
					continue
				}
				if claim.Classification != "dated-live-observation" || claim.Citation != citation {
					t.Errorf("evidence claim %s = %#v, want dated-live-observation cited to %s", claimPointer, claim, citation)
				}
			}
		}
	}
	if evidence.MaterialClaimCount != len(evidence.Claims) {
		t.Errorf("materialClaimCount = %d, want mechanically counted %d claims", evidence.MaterialClaimCount, len(evidence.Claims))
	}
}
