package app_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/KalebCole/partiful-cli/internal/app"
)

var approvedCommandCatalog = []string{
	"auth.login",
	"auth.logout",
	"auth.status",
	"blasts.send",
	"cohosts.invite",
	"cohosts.link.create",
	"cohosts.link.revoke",
	"cohosts.remove",
	"cohosts.revoke-invite",
	"contacts.list",
	"doctor",
	"events.cancel",
	"events.create",
	"events.get",
	"events.list",
	"events.update",
	"guests.invite",
	"guests.list",
	"posters.list",
	"posters.search",
	"rsvp.get",
	"rsvp.set",
	"schema",
	"version",
}

func TestExecuteSchemaPublishesEveryApprovedCommandDefinition(t *testing.T) {
	catalogResult := app.Execute(context.Background(), app.Request{Argv: []string{"schema"}}, app.Dependencies{})
	if catalogResult.ExitCode != 0 {
		t.Fatalf("schema exit code = %d, want 0", catalogResult.ExitCode)
	}
	var catalogEnvelope struct {
		Data struct {
			Commands []string `json:"commands"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(catalogResult.Stdout), &catalogEnvelope); err != nil {
		t.Fatalf("decode schema catalog: %v", err)
	}
	if !reflect.DeepEqual(catalogEnvelope.Data.Commands, approvedCommandCatalog) {
		t.Fatalf("schema commands = %v, want %v", catalogEnvelope.Data.Commands, approvedCommandCatalog)
	}

	for _, command := range approvedCommandCatalog {
		result := app.Execute(context.Background(), app.Request{Argv: []string{"schema", command}}, app.Dependencies{})
		if result.ExitCode != 0 {
			t.Fatalf("schema %s exit code = %d, want 0", command, result.ExitCode)
		}
		var envelope struct {
			Data struct {
				Command      string         `json:"command"`
				FailureTypes []string       `json:"failureTypes"`
				Safety       map[string]any `json:"safety"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
			t.Fatalf("decode schema %s: %v", command, err)
		}
		if envelope.Data.Command != command {
			t.Fatalf("schema command = %q, want %q", envelope.Data.Command, command)
		}
		if len(envelope.Data.FailureTypes) == 0 {
			t.Fatalf("schema %s omitted failure types", command)
		}
		if len(envelope.Data.Safety) == 0 {
			t.Fatalf("schema %s omitted safety metadata", command)
		}
	}
}

func TestExecuteVersionReportsBuildInfoContract(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{Argv: []string{"--version"}}, app.Dependencies{})
	if result.ExitCode != 0 {
		t.Fatalf("version exit code = %d, want 0", result.ExitCode)
	}
	var envelope struct {
		Data struct {
			Version                 string `json:"version"`
			ProductContractRevision string `json:"productContractRevision"`
			RemoteContractRevision  string `json:"remoteContractRevision"`
		} `json:"data"`
		Meta struct {
			Command                 string `json:"command"`
			CLIVersion              string `json:"cliVersion"`
			ProductContractRevision string `json:"productContractRevision"`
			RemoteContractRevision  string `json:"remoteContractRevision"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode version output: %v", err)
	}
	if envelope.Meta.Command != "version" {
		t.Fatalf("version meta command = %q, want version", envelope.Meta.Command)
	}
	if envelope.Data.Version != app.Version || envelope.Meta.CLIVersion != app.Version {
		t.Fatalf("version output = %q/%q, want %q", envelope.Data.Version, envelope.Meta.CLIVersion, app.Version)
	}
	if envelope.Data.ProductContractRevision != app.ProductContractRevision || envelope.Meta.ProductContractRevision != app.ProductContractRevision {
		t.Fatalf("product contract revision = %q/%q, want %q", envelope.Data.ProductContractRevision, envelope.Meta.ProductContractRevision, app.ProductContractRevision)
	}
	if envelope.Data.RemoteContractRevision != app.RemoteContractRevision || envelope.Meta.RemoteContractRevision != app.RemoteContractRevision {
		t.Fatalf("remote contract revision = %q/%q, want %q", envelope.Data.RemoteContractRevision, envelope.Meta.RemoteContractRevision, app.RemoteContractRevision)
	}
}
