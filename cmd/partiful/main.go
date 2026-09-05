package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/mcpserver"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

func main() {
	dependencies := productionDependencies()
	if len(os.Args) > 1 && os.Args[1] == "mcp" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runMCP(ctx, os.Args[2:], dependencies); err != nil && !errors.Is(err, context.Canceled) {
			_, _ = fmt.Fprintf(os.Stderr, "partiful mcp: %v\n", err)
			os.Exit(2)
		}
		return
	}
	result := app.Execute(context.Background(), app.Request{Argv: os.Args[1:], Stdin: os.Stdin}, dependencies)
	_, _ = io.WriteString(os.Stdout, result.Stdout)
	_, _ = io.WriteString(os.Stderr, result.Stderr)
	os.Exit(result.ExitCode)
}

func productionDependencies() app.Dependencies {
	credentialsPath, credentialsPathError := auth.DefaultCredentialsPath()
	cursorKeyPath, _ := app.DefaultCursorKeyPath()
	terminal := auth.OSTerminal{Input: os.Stdin, Output: os.Stderr}
	return app.Dependencies{
		Files: auth.OSFileSystem{}, CredentialsPath: credentialsPath, CredentialsPathError: credentialsPathError,
		Now: time.Now, HTTP: remote.NewHTTPClient(nil), CursorKeys: app.FileCursorKeyProvider{Path: cursorKeyPath},
		CursorRandom: rand.Reader, AuthRandom: rand.Reader, Terminal: terminal,
		Confirmer: terminalConfirmer{input: os.Stdin, output: os.Stderr},
	}
}

func runMCP(ctx context.Context, argv []string, dependencies app.Dependencies) error {
	if slices.Contains(argv, "-h") || slices.Contains(argv, "--help") {
		_, err := io.WriteString(os.Stdout, mcpserver.HelpText())
		return err
	}
	options, err := mcpserver.ParseOptions(argv)
	if err != nil {
		return err
	}
	if slices.Contains(argv, "--list-tools") {
		tools, err := mcpserver.EnabledTools(options)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(tools)
	}
	return mcpserver.Run(ctx, dependencies, options)
}
