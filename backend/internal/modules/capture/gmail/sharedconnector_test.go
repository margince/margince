// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// delegatedMailbox answers the profile of whoever holds the delegated token,
// with status when it is set.
func delegatedMailbox(t *testing.T, status int, bearers *[]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*bearers = append(*bearers, r.URL.Path+" "+r.Header.Get("Authorization"))
		if status != 0 {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"emailAddress": sharedMailbox, "historyId": "7"}); err != nil {
			t.Errorf("writing the profile: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sharedGmail(token, api *httptest.Server) *Connector {
	return New(nil, NewAPI(api.Client(), api.URL)).WithSharedMailboxes(SharedAuthConfig{TokenURL: token.URL, HTTPClient: token.Client()})
}

func TestASharedGmailProbeReadsTheProfileWithTheDelegatedToken(t *testing.T) {
	var forms []url.Values
	var bearers []string
	token := googleTokenEndpoint(t, http.StatusOK, `{"access_token":"mbx-tok","expires_in":3599}`, &forms)
	api := delegatedMailbox(t, 0, &bearers)
	auth := sharedAuth(t, serviceAccountKey(t), sharedMailbox)
	probed, err := sharedGmail(token, api).ProbeShared(context.Background(), auth)
	if err != nil {
		t.Fatal(err)
	}
	if string(probed) != string(auth) {
		t.Error("the probe changed the bundle it was given")
	}
	if len(bearers) != 1 || !strings.HasSuffix(bearers[0], "/profile Bearer mbx-tok") {
		t.Errorf("profile reads = %v, want one with the delegated token", bearers)
	}
}

func TestARefusedSharedGmailProbeNamesItsClass(t *testing.T) {
	key := serviceAccountKey(t)
	for name, tc := range map[string]struct {
		key, mailbox  string
		tokenStatus   int
		profileStatus int
		wantReason    string
	}{
		"no key file":      {"{}", sharedMailbox, http.StatusOK, 0, connector.SharedCredentialsInvalid},
		"no mailbox":       {key, "not an address", http.StatusOK, 0, connector.SharedCredentialsInvalid},
		"no delegation":    {key, sharedMailbox, http.StatusUnauthorized, 0, connector.SharedGrantRefused},
		"gmail turned off": {key, sharedMailbox, http.StatusOK, http.StatusForbidden, connector.SharedMailboxDenied},
	} {
		t.Run(name, func(t *testing.T) {
			var forms []url.Values
			var bearers []string
			token := googleTokenEndpoint(t, tc.tokenStatus, `{"error":"unauthorized_client","access_token":"mbx-tok","expires_in":3599}`, &forms)
			api := delegatedMailbox(t, tc.profileStatus, &bearers)
			_, err := sharedGmail(token, api).ProbeShared(context.Background(), sharedAuth(t, tc.key, tc.mailbox))
			var refusal *connector.SharedMailboxRefusal
			if !errors.As(err, &refusal) || refusal.Reason != tc.wantReason {
				t.Fatalf("err = %v, want a %s refusal", err, tc.wantReason)
			}
			if strings.Contains(err.Error(), "PRIVATE KEY") {
				t.Errorf("the refusal carries the key: %v", err)
			}
		})
	}
}

func TestAnOpenedSharedGmailConnectorActsForTheMailbox(t *testing.T) {
	var forms []url.Values
	var bearers []string
	token := googleTokenEndpoint(t, http.StatusOK, `{"access_token":"mbx-tok","expires_in":3599}`, &forms)
	api := delegatedMailbox(t, 0, &bearers)
	opened, state, err := sharedGmail(token, api).OpenShared(sharedAuth(t, serviceAccountKey(t), sharedMailbox))
	if err != nil {
		t.Fatal(err)
	}
	if label, err := opened.(*Connector).AccountLabel(state); err != nil || label != sharedMailbox {
		t.Errorf("AccountLabel = %q, %v; want the shared address", label, err)
	}
	if strings.Contains(string(state), "PRIVATE KEY") {
		t.Error("the run's auth state carries the key")
	}
	if err := opened.HealthCheck(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if len(forms) != 1 || len(bearers) != 1 || !strings.HasSuffix(bearers[0], "Bearer mbx-tok") {
		t.Errorf("token forms %d, reads %v; want one delegated mint and a read with it", len(forms), bearers)
	}
	if _, _, err := New(nil, nil).OpenShared(state); !errors.Is(err, errNoSharedReader) {
		t.Errorf("a connector with no shared reader opened a bundle: %v", err)
	}
}
