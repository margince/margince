// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmailsignature

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

const (
	readonlyScope = "https://www.googleapis.com/auth/gmail.readonly"
	sendScope     = "https://www.googleapis.com/auth/gmail.send"
)

// heldSignature is a mailbox whose provider answers one stored signature.
type heldSignature struct {
	html    string
	granted []string
	err     error
	reads   int
}

func (h *heldSignature) SignatureReaderFor(context.Context, ids.UserID, string) (connector.SendAsSignatureReader, connector.Auth, []string, error) {
	if h.err != nil {
		return nil, nil, nil, h.err
	}
	return h, connector.Auth(`{}`), h.granted, nil
}

func (h *heldSignature) SendAsSignature(context.Context, connector.Auth) (connector.SendAsSignature, error) {
	h.reads++
	return connector.SendAsSignature{Address: "rep@myco.com", HTML: h.html}, nil
}

func read(t *testing.T, source Source) crmcontracts.GmailSignatureImport {
	t.Helper()
	out, err := Read(context.Background(), source, ids.From[ids.UserKind](ids.NewV7()))
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestTheGmailSignatureIsAnsweredAsASavedSignatureKeepsIt(t *testing.T) {
	held := &heldSignature{
		html:    `<div><b>Ana Weber</b><br>Demo GmbH<script>alert(1)</script></div>`,
		granted: []string{readonlyScope},
	}
	out := read(t, held)
	if out.State != crmcontracts.GmailSignatureImportStateReady || out.Html == nil || out.Text == nil {
		t.Fatalf("answer = %+v, want a ready signature with markup and text", out)
	}
	if strings.Contains(*out.Html, "script") || !strings.Contains(*out.Html, "<b>Ana Weber</b>") {
		t.Errorf("markup = %q, want the bold name kept and the script dropped", *out.Html)
	}
	if *out.Text != "Ana Weber\nDemo GmbH" {
		t.Errorf("text = %q, want the plain-text form", *out.Text)
	}
	if out.Address == nil || *out.Address != "rep@myco.com" {
		t.Errorf("address = %v, want the send-as address", out.Address)
	}
}

func TestAGmailSignatureTheGrantCannotReadAsksToReconnect(t *testing.T) {
	held := &heldSignature{html: "<b>x</b>", granted: []string{sendScope}}
	if out := read(t, held); out.State != crmcontracts.GmailSignatureImportStateNeedsReconnect {
		t.Errorf("state = %q, want needs_reconnect", out.State)
	}
	if held.reads != 0 {
		t.Error("Gmail was asked although the grant cannot read the send-as settings")
	}
}

func TestNoMailboxAndAnEmptySignatureAreSaidSo(t *testing.T) {
	if out := read(t, nil); out.State != crmcontracts.GmailSignatureImportStateNoMailbox {
		t.Errorf("no registry: state = %q, want no_mailbox", out.State)
	}
	if out := read(t, &heldSignature{err: capture.ErrNoConnection}); out.State != crmcontracts.GmailSignatureImportStateNoMailbox {
		t.Errorf("no connection: state = %q, want no_mailbox", out.State)
	}
	if out := read(t, &heldSignature{html: "  ", granted: []string{readonlyScope}}); out.State != crmcontracts.GmailSignatureImportStateEmpty {
		t.Errorf("blank signature: state = %q, want empty", out.State)
	}
	imageOnly := `<img src="https://cdn.example.test/logo.png" width="120">`
	if out := read(t, &heldSignature{html: imageOnly, granted: []string{readonlyScope}}); out.State != crmcontracts.GmailSignatureImportStateEmpty {
		t.Errorf("image-only signature: state = %q, want empty, because a send skips it", out.State)
	}
}

// askedFor records whose connection the handler resolved.
type askedFor struct {
	heldSignature
	user     ids.UserID
	provider string
}

func (a *askedFor) SignatureReaderFor(ctx context.Context, user ids.UserID, provider string) (connector.SendAsSignatureReader, connector.Auth, []string, error) {
	a.user, a.provider = user, provider
	return a.heldSignature.SignatureReaderFor(ctx, user, provider)
}

func TestTheHandlerReadsOnlyTheSignedInHumansOwnGmail(t *testing.T) {
	me := ids.NewV7()
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, UserID: me})
	agent := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, UserID: me})

	source := &askedFor{html: "<b>Rep</b>", granted: []string{readonlyScope}}
	h := ImportHandlers{Source: source}
	w := httptest.NewRecorder()
	h.GetMyGmailSignature(w, httptest.NewRequestWithContext(human, http.MethodGet, "/v1/me/email-signature/gmail", nil))
	if w.Code != http.StatusOK || source.user.UUID != me || source.provider != "gmail" {
		t.Fatalf("status %d, read user %s provider %q; want 200 for the caller's own gmail", w.Code, source.user.UUID, source.provider)
	}

	refused := &askedFor{html: "<b>Rep</b>", granted: []string{readonlyScope}}
	w = httptest.NewRecorder()
	ImportHandlers{Source: refused}.GetMyGmailSignature(w, httptest.NewRequestWithContext(agent, http.MethodGet, "/v1/me/email-signature/gmail", nil))
	if w.Code != http.StatusForbidden || refused.reads != 0 {
		t.Errorf("an agent got status %d after %d reads, want 403 and none", w.Code, refused.reads)
	}
}
