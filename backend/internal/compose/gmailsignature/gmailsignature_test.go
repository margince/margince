// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmailsignature

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
}
