package main

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/KalebCole/partiful-cli/internal/app"
	"github.com/KalebCole/partiful-cli/internal/auth"
)

func main() {
	credentialsPath, credentialsPathError := auth.DefaultCredentialsPath()
	result := app.Execute(context.Background(), app.Request{
		Argv:  os.Args[1:],
		Stdin: os.Stdin,
	}, app.Dependencies{
		Files:                auth.OSFileSystem{},
		CredentialsPath:      credentialsPath,
		CredentialsPathError: credentialsPathError,
		Now:                  time.Now,
	})

	_, _ = io.WriteString(os.Stdout, result.Stdout)
	_, _ = io.WriteString(os.Stderr, result.Stderr)
	os.Exit(result.ExitCode)
}
