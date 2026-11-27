// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sentAsMailbox answers the app token and records the path and body of the
// one sendMail it receives.
type sentAsMailbox struct{ path, mime, bearer string }

func (s *sentAsMailbox) serve(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			writeJSON(w, map[string]any{"access_token": "app-tok", "expires_in": 3599})
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("reading the send body: %v", err)
		}
		raw, err := base64.StdEncoding.DecodeString(string(body))
		if err != nil {
			t.Errorf("the send body is not base64 MIME: %v", err)
		}
		s.path, s.mime, s.bearer = r.URL.EscapedPath(), string(raw), r.Header.Get("Authorization")
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestASharedSenderSendsFromTheMailboxWithTheAppToken(t *testing.T) {
	sent := &sentAsMailbox{}
	srv := sent.serve(t)
	sender, state, err := sharedConnector(srv).OpenSharedSender(sharedBundle(t, "contoso.example"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = sender.SendEmail(context.Background(), state, connector.EmailMessage{
		To: []string{"buyer@buyer.example"}, Subject: "Re: order", Body: "Shipped.", FromName: "Rep Example",
		MessageID: "reply-1@margince.test",
	})
	if err != nil {
		t.Fatalf("the shared send failed: %v", err)
	}
	if sent.path != "/users/"+sharedAddress+"/sendMail" || sent.bearer != "Bearer app-tok" {
		t.Errorf("the send went to %q with %q, want the mailbox's sendMail under the app token", sent.path, sent.bearer)
	}
	if !strings.Contains(sent.mime, "From: ") || !strings.Contains(sent.mime, "<"+sharedAddress+">") || !strings.Contains(sent.mime, "Rep Example") {
		t.Errorf("the message reads:\n%s\nwant From naming the shared address with the sender's display name", sent.mime)
	}
}

func TestAReadOnlySharedOpeningNeverSends(t *testing.T) {
	sent := &sentAsMailbox{}
	srv := sent.serve(t)
	reader, state, err := sharedConnector(srv).OpenShared(sharedBundle(t, "contoso.example"))
	if err != nil {
		t.Fatal(err)
	}
	sender, ok := reader.(connector.EmailSender)
	if !ok {
		t.Fatal("the opened connector must still be the Graph connector")
	}
	_, err = sender.SendEmail(context.Background(), state, connector.EmailMessage{
		To: []string{"buyer@buyer.example"}, Subject: "Re: order", Body: "Shipped.", MessageID: "reply-2@margince.test",
	})
	if err == nil || sent.path != "" {
		t.Fatalf("a read-only shared opening sent (path %q, error %v), want the send scope refusal", sent.path, err)
	}
}
