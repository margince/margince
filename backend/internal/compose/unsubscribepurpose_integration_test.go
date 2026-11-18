// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A link names the purpose its credential stops, and a press on it stops
// something or says it did not. Driven from the send the recipient receives,
// through the header URL, to the stored stop.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// sentHeaderURL sends one marketing message under purposeKey through the tool
// surface's seam and returns the one-click URL its List-Unsubscribe header
// carries.
func (c *carryEnv) sentHeaderURL(t *testing.T, address, purposeKey string) *url.URL {
	t.Helper()
	anchorID := ids.NewV7()
	if err := database.WithWorkspaceTx(c.admin, c.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'Monthly news', now(), 'manual', 'human:x')`, anchorID)
		return err
	}); err != nil {
		t.Fatalf("seeding the anchor: %v", err)
	}
	stager := &recordingStager{}
	adapter := newCommsAdapter(c.e.Pool, nil, SendPath{PublicBaseURL: toolSurfaceBaseURL, Delivery: stager})
	if _, err := adapter.SendEmail(c.admin, anchorID, agents.SendEmailArgs{
		To: []string{address}, Subject: "Monthly news", Body: "Here is the news.",
		ConsentPurpose: purposeKey,
	}); err != nil {
		t.Fatalf("the marketing send: %v", err)
	}
	if len(stager.staged) != 1 {
		t.Fatalf("staged %d deliveries, want 1", len(stager.staged))
	}
	header := strings.Trim(stager.staged[0].ListUnsubscribe, "<>")
	link, err := url.Parse(header)
	if err != nil || header == "" {
		t.Fatalf("the List-Unsubscribe header %q is not a URL: %v", header, err)
	}
	return link
}

// pressURL presses a one-click URL the way a mailbox provider does: the token
// and the query string it carries, nothing else, on the public principal.
func (c *carryEnv) pressURL(t *testing.T, link *url.URL) (int, string) {
	t.Helper()
	token := strings.TrimSuffix(strings.TrimPrefix(link.Path, "/v1/public/preferences/"), "/unsubscribe")
	var params crmcontracts.OneClickUnsubscribeParams
	if link.Query().Has("purpose") {
		named := link.Query().Get("purpose")
		params.Purpose = &named
	}
	return c.press(t, token, params)
}

func (c *carryEnv) press(t *testing.T, token string, params crmcontracts.OneClickUnsubscribeParams) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/public/preferences/"+token+"/unsubscribe",
		strings.NewReader("List-Unsubscribe=One-Click")).WithContext(c.publicCtx())
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	consent.NewHandlers(c.e.DB()).OneClickUnsubscribe(rec, req, token, params)
	return rec.Code, rec.Body.String()
}

// A send with no named purpose carries a link that names none, and its press
// stops the contact's marketing.
func TestAPurposeFreeSendsLinkStopsTheContactsMarketing(t *testing.T) {
	c := setupCarry(t)
	contact := c.contactAt(t, "Purpose Free", "purpose-free@unsub.test")
	newsletter := c.grantNewsletter(t, contact)

	link := c.sentHeaderURL(t, "purpose-free@unsub.test", "")
	if link.Query().Has("purpose") {
		t.Fatalf("the header %q names a purpose the send never had", link)
	}
	if status, body := c.pressURL(t, link); status != http.StatusOK || !strings.Contains(body, "stopped") {
		t.Fatalf("the press answered %d %s, want 200 with one stop", status, body)
	}
	if err := c.sendAllowed(newsletter, "purpose-free@unsub.test"); err == nil {
		t.Fatal("the newsletter still sends after the purpose-free link was pressed")
	}
}

// A lead holds no consent state, so the same link records a stop instead.
func TestAPurposeFreeSendsLinkStopsTheLeadsMarketing(t *testing.T) {
	c := setupCarry(t)
	lead := c.lead(t, "Lena Purpose Free", "lead-free@unsub.test", "Free GmbH")

	token, purpose, ok, err := preferenceLinkAdapter{store: c.consent}.UnsubscribeToken(c.admin, "lead-free@unsub.test", "")
	if err != nil || !ok || purpose != "" {
		t.Fatalf("minting for the lead: token ok=%v purpose=%q err=%v, want a purpose-free credential", ok, purpose, err)
	}
	if status, body := c.press(t, token, crmcontracts.OneClickUnsubscribeParams{}); status != http.StatusOK ||
		!strings.Contains(body, "stopped") {
		t.Fatalf("the lead's press answered %d %s, want 200 with one stop", status, body)
	}
	if n := c.count(t, `SELECT count(*) FROM communication_suppression
		 WHERE lead_id = $1 AND purpose_id IS NULL AND revoked_at IS NULL`, lead); n != 1 {
		t.Fatalf("the lead holds %d all-marketing stop(s) after the press, want 1", n)
	}
}

// A key the catalog never carried mints an all-marketing credential. The link
// must not echo that key. A press of an older link that did must stop all
// marketing, not stop nothing and answer success.
func TestAnUnknownPurposeKeyStopsAllMarketing(t *testing.T) {
	c := setupCarry(t)
	contact := c.contactAt(t, "Unknown Key", "unknown-key@unsub.test")
	newsletter := c.grantNewsletter(t, contact)

	token, purpose, ok, err := preferenceLinkAdapter{store: c.consent}.UnsubscribeToken(c.admin, "unknown-key@unsub.test", "no_such_purpose")
	if err != nil || !ok {
		t.Fatalf("minting: ok=%v err=%v", ok, err)
	}
	if purpose != "" {
		t.Fatalf("the mint answered purpose %q for an all-marketing credential, want none", purpose)
	}
	stale := "no_such_purpose"
	if status, body := c.press(t, token, crmcontracts.OneClickUnsubscribeParams{Purpose: &stale}); status != http.StatusOK ||
		!strings.Contains(body, "stopped") {
		t.Fatalf("the older link's press answered %d %s, want 200 with one stop", status, body)
	}
	if err := c.sendAllowed(newsletter, "unknown-key@unsub.test"); err == nil {
		t.Fatal("the newsletter still sends after a press that answered success")
	}
}

// A named credential stops its own purpose. Its link carries the key, and a
// press naming a different purpose is refused rather than widened.
func TestANamedLinkCannotBeWidened(t *testing.T) {
	c := setupCarry(t)
	contact := c.contactAt(t, "Named Link", "named@unsub.test")
	newsletter := c.grantNewsletter(t, contact)
	other := "carry_offers"
	source, wording := "manual", "Our offers."
	if _, err := c.consent.Record(c.admin, consent.RecordInput{
		ContactID: contact, PurposeID: c.purpose(t, other, false), NewState: "granted",
		Source: &source, PolicyText: &wording,
	}); err != nil {
		t.Fatalf("granting the offers: %v", err)
	}

	link := c.sentHeaderURL(t, "named@unsub.test", newsletter)
	if got := link.Query().Get("purpose"); got != newsletter {
		t.Fatalf("the header names purpose %q, want %q", got, newsletter)
	}
	token := strings.TrimSuffix(strings.TrimPrefix(link.Path, "/v1/public/preferences/"), "/unsubscribe")
	if status, _ := c.press(t, token, crmcontracts.OneClickUnsubscribeParams{Purpose: &other}); status == http.StatusOK {
		t.Fatal("a named link stopped a purpose it was not minted for")
	}
	// A press naming nothing is how a widening would arrive: it must stop only
	// the credential's own purpose.
	if status, _ := c.press(t, token, crmcontracts.OneClickUnsubscribeParams{}); status != http.StatusOK {
		t.Fatalf("the named link's own press answered %d", status)
	}
	if err := c.sendAllowed(newsletter, "named@unsub.test"); err == nil {
		t.Fatal("the newsletter still sends after its own link was pressed")
	}
	if err := c.sendAllowed(other, "named@unsub.test"); err != nil {
		t.Fatalf("the offers stopped too (%v): the named link widened to all marketing", err)
	}
}
