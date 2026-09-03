// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The relay a test reads the confirm-details link out of.
//
// Issuance of a double-opt-in token was withdrawn: the endpoint refuses, nothing
// writes consent_doi_token, and the redemption arm is gone. A purpose requiring
// double opt-in now confirms through MailboxProof alone, earned by SPENDING a
// link that was mailed to the subject's own live primary address.
//
// So a test needing a granted marketing consent has to do what the subject does,
// and the plaintext token is deliberately absent from every operator-facing
// response — which means there is no shortcut for a test either. Reading it out
// of the delivered message is the same access the subject has and no more; a
// helper that reached past the mail would assert a confirmation nobody performed,
// which is the exact defect the endpoint was withdrawn for.

import (
	"context"
	"regexp"
	"strings"
	"sync"
	"testing"
)

// capturingMailer stands in for the operator's outbound relay — a real boundary,
// which is the one kind of thing this repo's tests replace with a double.
type capturingMailer struct {
	mu   sync.Mutex
	body string
	sent int
}

func (m *capturingMailer) Send(_ context.Context, _, _, textBody string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.body, m.sent = textBody, m.sent+1
	return nil
}

// confirmLinkToken returns the token from the most recent confirm mail.
func (m *capturingMailer) confirmLinkToken(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sent == 0 {
		t.Fatal("no confirm mail was sent, so there is no link for the subject to spend")
	}
	link := regexp.MustCompile(`https?://\S+`).FindString(m.body)
	if link == "" {
		t.Fatalf("the confirm mail carries no link: %q", m.body)
	}
	token := link[strings.LastIndex(link, "/")+1:]
	if token == "" {
		t.Fatalf("the link carries no token: %q", link)
	}
	return token
}
