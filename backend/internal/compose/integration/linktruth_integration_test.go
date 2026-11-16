// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a confirm link's row says happened, and whether it is true.
//
// opened_at is evidence: the middle of the ask-to-click chain a controller
// produces to show a consent was freely given — the mail went out, the contact
// opened it, the answer landed.
//
// It was stamped by every GET, and most GETs of a link in a mail are machines.
// Mail security products fetch every link before delivering the message, and
// each of those wrote a line saying a named data subject opened their consent
// link.

import (
	"context"
	"net/http"
	"testing"
)

// linkOpened is when the newest token row says its link was opened, nil when
// it records no opening.
func linkOpened(t *testing.T, c *consentEnv) *string {
	t.Helper()
	var opened *string
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT opened_at::text FROM confirm_token ORDER BY issued_at DESC LIMIT 1`).
		Scan(&opened); err != nil {
		t.Fatalf("reading the link row: %v", err)
	}
	return opened
}

// askForALink has the workspace mail a confirm link and returns its token.
func askForALink(t *testing.T, c *consentEnv) string {
	t.Helper()
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/confirm-request",
		AnyMap{}, nil, nil); status != http.StatusCreated {
		t.Fatalf("ask the workspace to mail the confirm link → %d", status)
	}
	return confirmLinkToken(t, c.AppEnv)
}

// A SCANNER FETCHING THE LINK IS NOT THE SUBJECT OPENING IT.
//
// Mail security products fetch every link in a message before delivering it.
// Each of those used to write a line saying a named data subject opened their
// consent link — frequently at a moment they were asleep, and always in the
// direction that flatters the controller.
func TestAScannerFetchingTheLinkIsNotAnOpening(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	token := askForALink(t, c)

	if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
		map[string]string{"Sec-Purpose": "prefetch"}, nil); status != http.StatusOK {
		t.Fatalf("the scanner's fetch → %d, want 200 — the page still has to serve", status)
	}

	if linkOpened(t, c) != nil {
		t.Error("a prefetch was recorded as the subject opening their consent link, which puts " +
			"a moment that did not happen into the evidence for their grant")
	}
}

// A HUMAN OPENING THE PAGE STILL RECORDS AN OPENING.
//
// Without this, the test above would pass against an implementation that
// stopped writing opened_at altogether — which would lose the middle of the
// chain rather than making it truthful.
func TestSomebodyOpeningTheLinkIsRecordedAsOne(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	token := askForALink(t, c)

	if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
		map[string]string{"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"},
		nil); status != http.StatusOK {
		t.Fatalf("the subject opening their link → %d, want 200", status)
	}

	if linkOpened(t, c) == nil {
		t.Error("somebody navigated to their own confirm page and it was not recorded as an " +
			"opening, so the ask-to-click chain has no middle")
	}
}

// Repeated scanner fetches do not add up to an opening.
func TestRepeatedScannerFetchesNeverBecomeAnOpening(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	token := askForALink(t, c)

	scanner := map[string]string{"Sec-Purpose": "prefetch"}
	for range 4 {
		if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
			scanner, nil); status != http.StatusOK {
			t.Fatalf("fetching the link → %d", status)
		}
	}
	if linkOpened(t, c) != nil {
		t.Error("four machine fetches produced an opening")
	}
}
