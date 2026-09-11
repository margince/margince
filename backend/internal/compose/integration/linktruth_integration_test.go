// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a confirm link's row says happened, and whether it is true.
//
// opened_at is evidence: the middle of the ask-to-click chain a controller
// produces to show a consent was freely given — the mail went out, the person
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

// linkRow is what the token row records about being fetched.
type linkRow struct {
	opened       *string
	firstFetched *string
	fetchCount   int
}

func readLinkRow(t *testing.T, c *consentEnv) linkRow {
	t.Helper()
	var row linkRow
	if err := c.Owner.QueryRow(context.Background(), `
		SELECT opened_at::text, first_fetched_at::text, fetch_count
		  FROM confirm_token ORDER BY issued_at DESC LIMIT 1`).
		Scan(&row.opened, &row.firstFetched, &row.fetchCount); err != nil {
		t.Fatalf("reading the link row: %v", err)
	}
	return row
}

// askForALink has the workspace mail a confirm link and returns its token.
func askForALink(t *testing.T, c *consentEnv) string {
	t.Helper()
	if status := c.Call(t, "POST", "/v1/people/"+c.personID+"/consent/confirm-request",
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
	c := setupConsent(t)
	token := askForALink(t, c)

	if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
		map[string]string{"Sec-Purpose": "prefetch"}, nil); status != http.StatusOK {
		t.Fatalf("the scanner's fetch → %d, want 200 — the page still has to serve", status)
	}

	row := readLinkRow(t, c)
	if row.opened != nil {
		t.Error("a prefetch was recorded as the subject opening their consent link, which puts " +
			"a moment that did not happen into the evidence for their grant")
	}
	// THE FETCH IS STILL RECORDED, which is the other half: a link that is
	// fetched and never answered says something operationally, and dropping the
	// fact entirely would trade one untruth for a blind spot.
	if row.firstFetched == nil {
		t.Error("the fetch went unrecorded entirely, so nothing distinguishes a link nobody " +
			"received from one nobody answered")
	}
	if row.fetchCount != 1 {
		t.Errorf("fetch_count reads %d, want 1", row.fetchCount)
	}
}

// A PERSON OPENING THE PAGE STILL RECORDS AN OPENING.
//
// Without this, the test above would pass against an implementation that
// stopped writing opened_at altogether — which would lose the middle of the
// chain rather than making it truthful.
func TestSomebodyOpeningTheLinkIsRecordedAsOne(t *testing.T) {
	c := setupConsent(t)
	token := askForALink(t, c)

	if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
		map[string]string{"Sec-Fetch-Mode": "navigate", "Sec-Fetch-Dest": "document"},
		nil); status != http.StatusOK {
		t.Fatalf("the subject opening their link → %d, want 200", status)
	}

	row := readLinkRow(t, c)
	if row.opened == nil {
		t.Error("somebody navigated to their own confirm page and it was not recorded as an " +
			"opening, so the ask-to-click chain has no middle")
	}
	if row.firstFetched == nil {
		t.Error("the fetch was not recorded")
	}
}

// A SECOND FETCH COUNTS AND DOES NOT MOVE THE FIRST.
func TestRepeatedFetchesCountWithoutRewritingTheFirst(t *testing.T) {
	c := setupConsent(t)
	token := askForALink(t, c)

	scanner := map[string]string{"Sec-Purpose": "prefetch"}
	for range 3 {
		if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
			scanner, nil); status != http.StatusOK {
			t.Fatalf("fetching the link → %d", status)
		}
	}

	row := readLinkRow(t, c)
	// THE FIRST FETCH IS NOT MOVED by the ones after it. A column that took the
	// latest fetch would say the link was first seen at whatever moment the
	// most recent scanner happened to run, which is a different fact wearing
	// the same name.
	if row.firstFetched == nil {
		t.Fatal("no first fetch was recorded")
	}
	first := *row.firstFetched
	if status := publicCall(t, c.AppEnv, "GET", "/v1/public/confirm/"+token, nil,
		scanner, nil); status != http.StatusOK {
		t.Fatalf("a fourth fetch → %d", status)
	}
	if again := readLinkRow(t, c); again.firstFetched == nil || *again.firstFetched != first {
		t.Errorf("first_fetched_at moved from %q to %v on a later fetch", first, again.firstFetched)
	}
	row = readLinkRow(t, c)
	if row.fetchCount != 4 {
		t.Errorf("fetch_count reads %d after four fetches, want 4 — a link fetched many times "+
			"in seconds was not read that many times by a human, and the count is what says so",
			row.fetchCount)
	}
	if row.opened != nil {
		t.Error("three machine fetches produced an opening")
	}
}
