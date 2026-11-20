// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package sharedmailboxhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A shared mailbox belongs to no member, so its IMAP credential never carries
// SMTP settings, and with them the send scope.
func TestASharedIMAPMailboxRefusesSendSettings(t *testing.T) {
	probed := 0
	h := Handlers{ProbeIMAP: func(context.Context, http.ResponseWriter, *http.Request, *crmcontracts.ImapCredentials) (connector.Auth, bool) {
		probed++
		return connector.Auth("{}"), true
	}}
	secret := "app"
	req := crmcontracts.ConnectSharedMailboxRequest{
		Provider: crmcontracts.ConnectSharedMailboxRequestProviderImap,
		Imap: &crmcontracts.ImapCredentials{
			Host: "imap.example.test", Username: "sales@example.test", Secret: &secret,
			Smtp: &crmcontracts.ImapSmtpSettings{},
		},
	}
	w := httptest.NewRecorder()
	if _, ok := h.sharedCredential(w, httptest.NewRequest(http.MethodPost, "/", nil), req); ok {
		t.Fatal("a shared mailbox was connected with send settings")
	}
	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "shared_mailbox_cannot_send") || probed != 0 {
		t.Errorf("got %d %s after %d probes, want 422 shared_mailbox_cannot_send before any login", w.Code, w.Body.String(), probed)
	}

	req.Imap.Smtp = nil
	if _, ok := h.sharedCredential(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil), req); !ok || probed != 1 {
		t.Errorf("a read-only shared mailbox was refused (probed %d)", probed)
	}
}
