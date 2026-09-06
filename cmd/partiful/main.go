package main

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

func main() {
	credentialsPath, credentialsPathError := auth.DefaultCredentialsPath()
	cursorKeyPath, _ := app.DefaultCursorKeyPath()
	terminal := auth.OSTerminal{
		Input:  os.Stdin,
		Output: os.Stderr,
	}
	result := app.Execute(context.Background(), app.Request{
		Argv:  os.Args[1:],
		Stdin: os.Stdin,
	}, app.Dependencies{
		Files:                auth.OSFileSystem{},
		CredentialsPath:      credentialsPath,
		CredentialsPathError: credentialsPathError,
		Now:                  time.Now,
		HTTP:                 remote.NewHTTPClient(nil),
		CursorKeys:           app.FileCursorKeyProvider{Path: cursorKeyPath},
		CursorRandom:         rand.Reader,
		AuthRandom:           rand.Reader,
		Terminal:             terminal,
		Confirmer: terminalConfirmer{
			input:  os.Stdin,
			output: os.Stderr,
		},
	})

	_, _ = io.WriteString(os.Stdout, result.Stdout)
	_, _ = io.WriteString(os.Stderr, result.Stderr)
	os.Exit(result.ExitCode)
}
