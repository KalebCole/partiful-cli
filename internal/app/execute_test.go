package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
)

func TestExecuteVersion(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--version"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":true,"data":{"version":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"},"meta":{"command":"version","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteRejectsUnknownCommand(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"events", "list"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_NOT_FOUND","message":"Unknown command.","retryable":false,"details":{}},"meta":{"command":"unknown","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePrettyPrintsOneCompleteEnvelope(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--pretty", "--version"},
		Stdin: strings.NewReader(""),
	})

	const want = `{
  "ok": true,
  "data": {
    "version": "1.0.0",
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1"
  },
  "meta": {
    "command": "version",
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1",
    "warnings": []
  }
}
`
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteAcceptsNonInteractiveGlobalFlag(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--version", "--non-interactive"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":true,"data":{"version":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"},"meta":{"command":"version","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteRejectsRepeatedScalarFlag(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"--pretty", "--version", "--pretty"},
		Stdin: strings.NewReader(""),
	})

	const want = `{
  "ok": false,
  "error": {
    "type": "input.invalid",
    "code": "FLAG_REPEATED",
    "message": "A scalar flag cannot be repeated.",
    "retryable": false,
    "details": {
      "flag": "--pretty"
    }
  },
  "meta": {
    "command": "version",
    "cliVersion": "1.0.0",
    "productContractRevision": "2026-08-10.1",
    "remoteContractRevision": "2026-08-10.1"
  }
}
`
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteSchemaListsOnlyCompletedCatalog(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":true,"data":{"commands":["auth.logout","auth.status","doctor","schema","version"]},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteSchemaProjectsExecutableDefinition(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "auth.status"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":true,"data":{"command":"auth.status","positionals":[],"flags":[],"inputSchema":{"type":"object","additionalProperties":false},"successSchema":{"type":"object","additionalProperties":false,"required":["authenticated","tokenState","expiresAt"],"properties":{"authenticated":{"type":"boolean"},"expiresAt":{"type":["string","null"],"format":"date-time"},"tokenState":{"type":"string","enum":["healthy","expiring","expired","missing"]}}},"failureTypes":["internal.failure"],"safety":{"kind":"read-only","planRequired":false,"confirmationRequired":false}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteRejectsUnknownSchemaPathWithoutEchoingInput(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "secret-private-value"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_SCHEMA_NOT_FOUND","message":"No completed command has that schema path.","retryable":false,"details":{}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, "secret-private-value") {
		t.Fatal("output echoed untrusted command input")
	}
}

func TestExecuteAuthStatusWhenCredentialsAreMissing(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return nil, fs.ErrNotExist
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"missing","expiresAt":null},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

type fakeFilesystem struct {
	readFile func(string) ([]byte, error)
	remove   func(string) error
}

func (filesystem fakeFilesystem) ReadFile(path string) ([]byte, error) {
	return filesystem.readFile(path)
}

func (filesystem fakeFilesystem) Remove(path string) error {
	if filesystem.remove != nil {
		return filesystem.remove(path)
	}
	return nil
}

func TestExecuteAuthStatusRedactsHealthyCredentials(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","refreshToken":"secret-refresh-value","userId":"private-user-value","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"healthy","expiresAt":"2026-08-11T02:00:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	for _, privateValue := range []string{"secret-token-value", "secret-refresh-value", "private-user-value"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("output contains private credential value %q", privateValue)
		}
	}
}

func TestExecuteAuthStatusReportsExpiringToken(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-11T00:04:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":true,"tokenState":"expiring","expiresAt":"2026-08-11T00:04:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteAuthStatusReportsExpiredToken(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-10T23:59:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"expired","expiresAt":"2026-08-10T23:59:00Z"},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteAuthStatusFailureDoesNotRevealCredentialContents(t *testing.T) {
	const privateContents = "secret-token-content private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(privateContents), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIALS_INVALID","message":"Local credentials are invalid.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	const wantStderr = "partiful: local operation failed\n"
	if result.ExitCode != 10 {
		t.Fatalf("exit code = %d, want 10", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if result.Stderr != wantStderr {
		t.Fatalf("stderr = %q, want %q", result.Stderr, wantStderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateContents) {
		t.Fatal("failure output revealed credential file contents")
	}
}

func TestExecuteAuthLogoutAtomicallyRemovesCredentials(t *testing.T) {
	const credentialsPath = "/config/partiful/credentials.json"
	files := &memoryFilesystem{
		files: map[string][]byte{
			credentialsPath: []byte(`{"accessToken":"secret-token-value","expiresAt":"2026-08-11T02:00:00Z"}`),
		},
	}
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: credentialsPath,
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "logout"},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const want = `{"ok":true,"data":{"authenticated":false,"tokenState":"missing","expiresAt":null},"meta":{"command":"auth.logout","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}

	status := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	if !strings.Contains(status.Stdout, `"authenticated":false,"tokenState":"missing","expiresAt":null`) {
		t.Fatalf("status after logout = %q, want missing credentials", status.Stdout)
	}
}

type memoryFilesystem struct {
	files map[string][]byte
}

func (filesystem *memoryFilesystem) ReadFile(path string) ([]byte, error) {
	document, ok := filesystem.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), document...), nil
}

func (filesystem *memoryFilesystem) Remove(path string) error {
	if _, ok := filesystem.files[path]; !ok {
		return fs.ErrNotExist
	}
	delete(filesystem.files, path)
	return nil
}

func TestExecuteAuthLogoutFailureLeavesCredentialsAvailableAndRedactsError(t *testing.T) {
	const privateValue = "private-user-identifier"
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-11T02:00:00Z"}`
	files := fakeFilesystem{
		readFile: func(string) ([]byte, error) {
			return []byte(credentials), nil
		},
		remove: func(string) error {
			return errors.New("filesystem failure involving " + privateValue)
		},
	}
	dependencies := app.Dependencies{
		Files:           files,
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	}

	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "logout"},
		Stdin: strings.NewReader(""),
	}, dependencies)

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CREDENTIAL_STORE_UNAVAILABLE","message":"Local credential storage is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.logout","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	if result.ExitCode != 10 {
		t.Fatalf("exit code = %d, want 10", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateValue) {
		t.Fatal("logout failure output revealed a private identifier")
	}

	status := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, dependencies)
	if !strings.Contains(status.Stdout, `"authenticated":true,"tokenState":"healthy"`) {
		t.Fatalf("status after failed logout = %q, want credentials preserved", status.Stdout)
	}
}

func TestExecuteDoctorReportsHealthyCredentialsWithoutPrivateData(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","userId":"private-user-identifier","expiresAt":"2026-08-11T02:00:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":true,"checks":[{"name":"credentials","status":"pass","message":"Authentication credentials are available.","remediation":null}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	for _, privateValue := range []string{"secret-token-value", "private-user-identifier"} {
		if strings.Contains(result.Stdout+result.Stderr, privateValue) {
			t.Fatalf("doctor output contains private value %q", privateValue)
		}
	}
}

func TestExecuteDoctorReportsMissingCredentialsAsARedactedCheck(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return nil, fs.ErrNotExist
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials are missing.","remediation":"Establish authentication before using commands that require it."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteDoctorWarnsWhenCredentialsExpireSoon(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-11T00:04:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":true,"checks":[{"name":"credentials","status":"warn","message":"Authentication credentials expire soon.","remediation":"Refresh authentication before the credentials expire."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteDoctorFailsExpiredCredentialsCheck(t *testing.T) {
	const credentials = `{"accessToken":"secret-token-value","expiresAt":"2026-08-10T23:59:00Z"}`
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(credentials), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials have expired.","remediation":"Re-establish authentication."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteDoctorRedactsInvalidCredentialFile(t *testing.T) {
	const privateContents = "secret-token-content private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return []byte(privateContents), nil
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Authentication credentials are invalid.","remediation":"Remove the invalid credentials and re-establish authentication."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateContents) {
		t.Fatal("doctor output revealed credential file contents")
	}
}

func TestExecuteDoctorRedactsCredentialStorageFailure(t *testing.T) {
	const privateError = "permission failure for private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files: fakeFilesystem{
			readFile: func(string) ([]byte, error) {
				return nil, errors.New(privateError)
			},
		},
		CredentialsPath: "/config/partiful/credentials.json",
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Credential storage is unavailable.","remediation":"Check local credential file permissions."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("doctor output revealed filesystem error contents")
	}
}

func TestExecuteSchemaDescribesBothDiscoveryResultShapes(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "schema"},
		Stdin: strings.NewReader(""),
	})
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}

	var envelope struct {
		Data struct {
			SuccessSchema any `json:"successSchema"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(result.Stdout), &envelope); err != nil {
		t.Fatalf("decode stdout: %v", err)
	}

	const expectedLiteral = `{
		"type": "object",
		"oneOf": [
			{
				"type": "object",
				"additionalProperties": false,
				"required": ["commands"],
				"properties": {
					"commands": {"type": "array", "items": {"type": "string"}}
				}
			},
			{
				"type": "object",
				"additionalProperties": false,
				"required": ["command", "positionals", "flags", "inputSchema", "successSchema", "failureTypes", "safety"],
				"properties": {
					"command": {"type": "string"},
					"positionals": {
						"type": "array",
						"items": {
							"type": "object",
							"additionalProperties": false,
							"required": ["name", "required", "description"],
							"properties": {
								"name": {"type": "string"},
								"required": {"type": "boolean"},
								"description": {"type": "string"}
							}
						}
					},
					"flags": {
						"type": "array",
						"items": {
							"type": "object",
							"additionalProperties": false,
							"required": ["name", "required", "description"],
							"properties": {
								"name": {"type": "string"},
								"required": {"type": "boolean"},
								"description": {"type": "string"}
							}
						}
					},
					"inputSchema": {"type": "object"},
					"successSchema": {"type": "object"},
					"failureTypes": {"type": "array", "items": {"type": "string"}},
					"safety": {
						"type": "object",
						"additionalProperties": false,
						"required": ["kind", "planRequired", "confirmationRequired"],
						"properties": {
							"kind": {"type": "string", "enum": ["read-only", "local-mutation"]},
							"planRequired": {"type": "boolean"},
							"confirmationRequired": {"type": "boolean"}
						}
					}
				}
			}
		]
	}`
	var expected any
	if err := json.Unmarshal([]byte(expectedLiteral), &expected); err != nil {
		t.Fatalf("decode expected schema: %v", err)
	}
	if !reflect.DeepEqual(envelope.Data.SuccessSchema, expected) {
		t.Fatalf("success schema = %#v, want %#v", envelope.Data.SuccessSchema, expected)
	}
}

func TestExecuteUsesDefinitionForFlagFailureCommandMetadata(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status", "--non-interactive", "--non-interactive"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":false,"error":{"type":"input.invalid","code":"FLAG_REPEATED","message":"A scalar flag cannot be repeated.","retryable":false,"details":{"flag":"--non-interactive"}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecutePrettyAppliesWhenItFollowsInvalidGlobalFlag(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv: []string{
			"auth",
			"status",
			"--non-interactive",
			"--non-interactive",
			"--pretty",
		},
		Stdin: strings.NewReader(""),
	})

	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if !strings.HasPrefix(result.Stdout, "{\n  \"ok\": false,") {
		t.Fatalf("stdout = %q, want indented failure envelope", result.Stdout)
	}
	if !strings.Contains(result.Stdout, `"command": "auth.status"`) {
		t.Fatalf("stdout = %q, want command auth.status", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
}

func TestExecuteUsesKnownCommandMetadataForInvalidArity(t *testing.T) {
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"schema", "auth.status", "extra-private-value"},
		Stdin: strings.NewReader(""),
	})

	const want = `{"ok":false,"error":{"type":"usage.invalid","code":"COMMAND_NOT_FOUND","message":"Unknown command.","retryable":false,"details":{}},"meta":{"command":"schema","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	if result.ExitCode != 2 {
		t.Fatalf("exit code = %d, want 2", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if strings.Contains(result.Stdout+result.Stderr, "extra-private-value") {
		t.Fatal("invalid-arity output echoed untrusted input")
	}
}

func TestExecuteAuthStatusReportsConfigurationDirectoryFailure(t *testing.T) {
	const privateError = "configuration error containing private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"auth", "status"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files:                fakeFilesystem{},
		CredentialsPathError: errors.New(privateError),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const wantStdout = `{"ok":false,"error":{"type":"internal.failure","code":"CONFIG_DIRECTORY_UNAVAILABLE","message":"Local configuration directory is unavailable.","retryable":false,"details":{}},"meta":{"command":"auth.status","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1"}}` + "\n"
	if result.ExitCode != 10 {
		t.Fatalf("exit code = %d, want 10", result.ExitCode)
	}
	if result.Stdout != wantStdout {
		t.Fatalf("stdout = %q, want %q", result.Stdout, wantStdout)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("configuration failure output revealed private error contents")
	}
}

func TestExecuteDoctorDiagnosesConfigurationDirectoryFailure(t *testing.T) {
	const privateError = "configuration error containing private-user-identifier"
	result := app.Execute(context.Background(), app.Request{
		Argv:  []string{"doctor"},
		Stdin: strings.NewReader(""),
	}, app.Dependencies{
		Files:                fakeFilesystem{},
		CredentialsPathError: errors.New(privateError),
		Now: func() time.Time {
			return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC)
		},
	})

	const want = `{"ok":true,"data":{"healthy":false,"checks":[{"name":"credentials","status":"fail","message":"Configuration directory is unavailable.","remediation":"Set a usable user configuration directory."}]},"meta":{"command":"doctor","cliVersion":"1.0.0","productContractRevision":"2026-08-10.1","remoteContractRevision":"2026-08-10.1","warnings":[]}}` + "\n"
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Stdout != want {
		t.Fatalf("stdout = %q, want %q", result.Stdout, want)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if strings.Contains(result.Stdout+result.Stderr, privateError) {
		t.Fatal("doctor output revealed configuration error contents")
	}
}
