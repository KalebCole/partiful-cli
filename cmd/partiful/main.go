package main

import (
	"context"
	"crypto/rand"
	"io"
	"os"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/auth"
	cursorstate "github.com/KalebCole/partiful-cli/internal/cursor"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

func main() {
	credentialsPath, credentialsPathError := auth.DefaultCredentialsPath()
	cursorKeyPath, _ := cursorstate.DefaultKeyPath()
	result := app.Execute(context.Background(), app.Request{
		Argv:  os.Args[1:],
		Stdin: os.Stdin,
	}, app.Dependencies{
		Files:                auth.OSFileSystem{},
		CredentialsPath:      credentialsPath,
		CredentialsPathError: credentialsPathError,
		Now:                  time.Now,
		HTTP:                 remote.NewHTTPClient(nil),
		CursorKeys:           cursorstate.FileKeyProvider{Path: cursorKeyPath},
		CursorRandom:         rand.Reader,
	})

	_, _ = io.WriteString(os.Stdout, result.Stdout)
	_, _ = io.WriteString(os.Stderr, result.Stderr)
	os.Exit(result.ExitCode)
}
