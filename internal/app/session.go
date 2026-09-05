package app

import (
	"context"
	"errors"
	"time"

	"github.com/KalebCole/partiful-cli/internal/auth"
	"github.com/KalebCole/partiful-cli/internal/remote"
)

func acquireProtectedSession(
	ctx context.Context,
	command string,
	dependencies Dependencies,
	pretty bool,
) (auth.Session, *Result) {
	return acquireProtectedSessionWithPersistence(
		ctx,
		command,
		dependencies,
		pretty,
		!dependencies.ExecutionPolicy.DisableCredentialPersistence,
	)
}

func acquireProtectedMutationSession(
	ctx context.Context,
	command string,
	dependencies Dependencies,
	execution mutationExecution,
	pretty bool,
) (auth.Session, *Result) {
	return acquireProtectedSessionWithPersistence(
		ctx,
		command,
		dependencies,
		pretty,
		!execution.DryRun && !dependencies.ExecutionPolicy.DisableCredentialPersistence,
	)
}

func acquireProtectedSessionWithPersistence(
	ctx context.Context,
	command string,
	dependencies Dependencies,
	pretty bool,
	persistRefresh bool,
) (auth.Session, *Result) {
	if dependencies.CredentialsPathError != nil {
		result := configurationDirectoryFailure(command, pretty)
		return auth.Session{}, &result
	}
	clock := time.Now
	if dependencies.Now != nil {
		clock = dependencies.Now
	}
	acquire := auth.AcquireSession
	if !persistRefresh {
		acquire = auth.AcquireSessionWithoutPersistence
	}
	session, err := acquire(
		ctx,
		dependencies.Files,
		dependencies.CredentialsPath,
		clock,
		remote.AuthClient{HTTP: dependencies.HTTP},
	)
	if err == nil {
		return session, nil
	}
	var result Result
	switch {
	case errors.Is(err, auth.ErrRequired):
		result = authenticationRequiredFailure(command, pretty)
	case errors.Is(err, auth.ErrRemoteTokenExpired):
		result = authenticationExpiredFailure(
			command,
			"INVALID_REFRESH_TOKEN",
			"Stored authentication has expired. Log in again.",
			pretty,
		)
	case errors.Is(err, auth.ErrExpired):
		result = authenticationExpiredFailure(
			command,
			"SESSION_EXPIRED",
			"Stored authentication has expired. Log in again.",
			pretty,
		)
	case errors.Is(err, auth.ErrRemoteProtocolChanged):
		result = authenticationProtocolChangedFailure(command, pretty)
	case errors.Is(err, auth.ErrRemoteUnavailable):
		result = authenticationUnavailableFailure(command, pretty)
	case errors.Is(err, auth.ErrInvalid):
		result = credentialInvalidFailure(command, pretty)
	case errors.Is(err, auth.ErrUnavailable),
		errors.Is(err, auth.ErrPersistence):
		result = credentialUnavailableFailure(command, pretty)
	default:
		result = internalFailure(command, pretty)
	}
	return auth.Session{}, &result
}
