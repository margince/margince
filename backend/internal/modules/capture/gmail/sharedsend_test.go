// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func TestAMailboxReadsWithoutTheSendScopeAndSendsWithBoth(t *testing.T) {
	var forms []url.Values
	srv := googleTokenEndpoint(t, http.StatusOK, `{"access_token":"mbx-tok","expires_in":3599}`, &forms)
	clock := &sharedClock{at: time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)}
	authz := NewSharedAuthorizer(SharedAuthConfig{TokenURL: srv.URL, HTTPClient: srv.Client(), Now: clock.now})
	auth := sharedAuth(t, serviceAccountKey(t), sharedMailbox)

	if _, err := authz.AccessToken(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	if _, err := authz.SendAccessToken(context.Background(), auth); err != nil {
		t.Fatal(err)
	}
	if len(forms) != 2 {
		t.Fatalf("the token endpoint was asked %d times, want one per scope set", len(forms))
	}
	read := assertionClaims(t, forms[0].Get("assertion"))["scope"]
	send := assertionClaims(t, forms[1].Get("assertion"))["scope"]
	if read != "https://www.googleapis.com/auth/gmail.readonly" {
		t.Errorf("the read assertion asks for %v, want the read-only scope alone", read)
	}
	if send != "https://www.googleapis.com/auth/gmail.readonly "+SendScope {
		t.Errorf("the send assertion asks for %v, want the read scope and the send scope", send)
	}
}

func TestOnlyTheSenderOpeningNamesTheSendScopeAsGranted(t *testing.T) {
	connector := New(nil, nil).WithSharedMailboxes(SharedAuthConfig{})
	auth := sharedAuth(t, serviceAccountKey(t), sharedMailbox)

	_, readState, err := connector.OpenShared(auth)
	if err != nil {
		t.Fatal(err)
	}
	_, sendState, err := connector.OpenSharedSender(auth)
	if err != nil {
		t.Fatal(err)
	}
	granted := func(state []byte) []string {
		var st authState
		if err := json.Unmarshal(state, &st); err != nil {
			t.Fatal(err)
		}
		return st.Granted
	}
	if len(granted(readState)) != 0 {
		t.Errorf("the read opening names %v as granted, want none", granted(readState))
	}
	if got := granted(sendState); len(got) != 1 || got[0] != SendScope {
		t.Errorf("the send opening names %v as granted, want the send scope", got)
	}
}
