package remote

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type guestHTTPClientFunc func(*http.Request) (*http.Response, error)

func (do guestHTTPClientFunc) Do(request *http.Request) (*http.Response, error) {
	return do(request)
}

func TestInviteGuestsAsHostValidatesReviewedCompletionEnvelope(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "reviewed result object", body: `{"result":{"ok":true}}`},
		{name: "unreviewed data envelope", body: `{"data":{}}`, wantError: true},
		{name: "null result", body: `{"result":null}`, wantError: true},
		{name: "missing result", body: `{}`, wantError: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			client := Client{HTTP: guestHTTPClientFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(testCase.body)),
				}, nil
			})}
			err := client.InviteGuestsAsHost(
				context.Background(),
				"private-access-token",
				"private-user",
				"private-device",
				InviteGuestsAsHostParams{
					EventID:               "event-example",
					UserIDsToInvite:       []string{"private-contact"},
					InvitationMessage:     "",
					OtherMutualsCount:     0,
					PhoneContactsToInvite: []map[string]any{},
					EmailsToInvite:        []map[string]any{},
				},
			)
			if testCase.wantError && !errors.Is(err, ErrProtocolChanged) {
				t.Fatalf("error = %v, want protocol changed", err)
			}
			if !testCase.wantError && err != nil {
				t.Fatalf("error = %v, want success", err)
			}
		})
	}
}
