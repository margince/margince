// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	typedSubject = "Zugang Passwort: Hunter2-Example!"
	typedBody    = "Hallo Pat,\nPasswort: Hunter2-Example!\nBis bald"
	typedSecret  = "Hunter2-Example!"
)

func TestAMailSendStoresMaskedTextAndDeliversWhatWasTyped(t *testing.T) {
	m := outboundMessage{
		in:           SendEmailInput{Subject: typedSubject, Body: typedBody},
		messageID:    "outbound-1@margince.test",
		body:         typedBody,
		recordedBody: typedBody,
		to:           []string{"pat@counterparty.test"},
	}
	row := m.activity(threading{})
	assertMaskedText(t, "timeline subject", row.Subject, typedSubject)
	assertMaskedText(t, "timeline body", row.Body, typedBody)

	delivery := m.delivery(ids.NewV7(), threading{}, SendOrigin{})
	for field, got := range map[string]string{
		"subject": delivery.Subject, "body": delivery.Body,
		"authored subject": delivery.AuthoredSubject, "authored body": delivery.AuthoredBody,
		"judged body": delivery.Authorization.Body,
	} {
		if !strings.Contains(got, typedSecret) {
			t.Errorf("delivery %s = %q, want the text as typed", field, got)
		}
	}
}

func TestAChannelReplyStoresMaskedTextAndDeliversWhatWasTyped(t *testing.T) {
	m := outboundChannelMessage{in: SendMessageInput{Body: typedBody}, provider: "telegram"}
	assertMaskedText(t, "timeline body", m.activity().Body, typedBody)
	if got := m.delivery(ids.From[ids.ActivityKind](ids.NewV7())).Body; got != typedBody {
		t.Errorf("delivery body = %q, want the text as typed", got)
	}
}

// assertMaskedText holds a stored field to the text capture would store for
// the same typed text, so a sent row and its synced copy read alike.
func assertMaskedText(t *testing.T, field string, stored *string, typed string) {
	t.Helper()
	if stored == nil {
		t.Fatalf("%s is unset", field)
	}
	if want := credentialmask.MaskText(typed); *stored != want || !strings.Contains(*stored, credentialmask.Marker) {
		t.Errorf("%s = %q, want %q", field, *stored, want)
	}
}
