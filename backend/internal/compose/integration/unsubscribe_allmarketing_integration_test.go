// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A marketing send that names no catalog purpose carries an all-marketing
// withdrawal credential. Its links name no purpose, and a press on them stops
// all marketing, for a contact and for a lead alike.

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// marketingDelivery is what reached the recipient of one marketing send.
type marketingDelivery struct {
	header string // the List-Unsubscribe value
	page   string // the visible unsubscribe link
	manage string // the visible manage-preferences link
}

// deliverMarketing sends a marketing message and answers the delivery staged for
// it. A send naming no granted catalog purpose is refused, and a designated
// human then directs it, which is how such a message reaches a recipient.
func deliverMarketing(t *testing.T, c *consentEnv, to, marketingPurpose string, refused bool) marketingDelivery {
	t.Helper()
	body := AnyMap{"communication_context": "marketing", "subject": "News", "body": "hello", "to": []string{to}}
	if marketingPurpose != "" {
		body["marketing_purpose"] = marketingPurpose
	}
	want := http.StatusAccepted
	if refused {
		want = http.StatusConflict
	}
	if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", body, nil, nil); status != want {
		t.Fatalf("marketing send to %s → %d, want %d", to, status, want)
	}
	if refused {
		if status := c.Call(t, "POST", "/v1/communication-reviews/"+liveReviewID(t, c)+"/direct-send",
			directed(), nil, nil); status != http.StatusCreated {
			t.Fatalf("directing the send → %d, want 201", status)
		}
	}
	var delivered, header string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT body, list_unsubscribe FROM comms_outbound ORDER BY id DESC LIMIT 1`).Scan(&delivered, &header); err != nil {
		t.Fatalf("reading the staged delivery: %v", err)
	}
	return marketingDelivery{
		header: strings.Trim(header, "<>"),
		page:   unsubscribeLinkIn(t, delivered),
		manage: manageLinkIn(t, delivered),
	}
}

// assertNamesNoPurpose fails when either stop link of an all-marketing
// credential names a purpose.
func assertNamesNoPurpose(t *testing.T, d marketingDelivery) {
	t.Helper()
	if strings.Contains(d.header, "purpose=") || !strings.HasSuffix(d.header, "/unsubscribe") {
		t.Errorf("List-Unsubscribe = %q, want the one-click URL with no purpose", d.header)
	}
	token := tokenFromLink(t, d.page)
	if want := "https://mail.example.test/#/unsubscribe/" + token + "?lang=en"; d.page != want {
		t.Errorf("unsubscribe link = %q, want %q", d.page, want)
	}
}

// pressOneClick POSTs a stop URL the way a mailbox provider and the page both
// do, and answers how many purposes it reports stopped.
func pressOneClick(t *testing.T, c *consentEnv, oneClick string) int {
	t.Helper()
	path := strings.TrimPrefix(oneClick, "https://mail.example.test")
	var answer struct {
		Unsubscribed []string `json:"unsubscribed"`
	}
	if s := publicCall(t, c.AppEnv, "POST", path, nil, nil, &answer); s != http.StatusOK {
		t.Fatalf("POST %s → %d, want 200", path, s)
	}
	return len(answer.Unsubscribed)
}

// countRows answers a count the test reads to see whether anything was written.
func countRows(t *testing.T, c *consentEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := c.Owner.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

const withdrawalsOfMarketing = `
	SELECT count(*) FROM consent_event ce JOIN consent_purpose cp ON cp.id = ce.purpose_id
	 WHERE ce.new_state = 'withdrawn' AND cp.key = $1`

func TestAnAllMarketingLinkStopsAContactsMarketing(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	c.grantMarketingByConfirmLink(t)

	d := deliverMarketing(t, c, "subject@consent.test", "", true)
	assertNamesNoPurpose(t, d)

	// Opening the page reads, and a withdrawal credential may read nothing.
	stop := tokenFromLink(t, d.page)
	if s := publicCall(t, c.AppEnv, "GET", "/v1/public/preferences/"+stop, nil, nil, nil); s != http.StatusNotFound {
		t.Fatalf("the page's read with a withdrawal credential → %d, want 404", s)
	}
	if n := countRows(t, c, withdrawalsOfMarketing, "marketing_email"); n != 0 {
		t.Fatalf("opening the page wrote %d withdrawal(s), want none", n)
	}

	if n := pressOneClick(t, c, d.header); n != 1 {
		t.Fatalf("the press reported %d stopped purpose(s), want 1", n)
	}
	manage := manageTokenFromLink(t, d.manage)
	if s, _ := purposeStateOf(t, readPreferenceView(t, c, manage), "marketing_email"); s != "withdrawn" {
		t.Fatalf("marketing_email is %q after the all-marketing press, want withdrawn", s)
	}

	// A repeat confirmation changes nothing and says so.
	if n := pressOneClick(t, c, d.header); n != 0 {
		t.Errorf("the repeat press reported %d stopped purpose(s), want 0", n)
	}
	if n := countRows(t, c, withdrawalsOfMarketing, "marketing_email"); n != 1 {
		t.Errorf("%d withdrawal rows after a repeat press, want 1", n)
	}
}

func TestAnAllMarketingLinkStopsALeadsMarketing(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	var lead string
	if err := c.Owner.QueryRow(context.Background(),
		`INSERT INTO lead (full_name, email, source, captured_by)
		 VALUES ('Lead Reader', 'lead-reader@consent.test', 'test', 'human:x') RETURNING id::text`).Scan(&lead); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}

	d := deliverMarketing(t, c, "lead-reader@consent.test", "", true)
	assertNamesNoPurpose(t, d)

	const liveBroadStops = `
		SELECT count(*) FROM communication_suppression
		 WHERE lead_id = $1 AND purpose_id IS NULL AND revoked_at IS NULL`
	if n := countRows(t, c, liveBroadStops, lead); n != 0 {
		t.Fatalf("the lead holds %d stop(s) before any press, want none", n)
	}
	if n := pressOneClick(t, c, d.header); n != 1 {
		t.Fatalf("the press reported %d stop(s), want 1", n)
	}
	if n := countRows(t, c, liveBroadStops, lead); n != 1 {
		t.Fatalf("the lead holds %d all-marketing stop(s) after the press, want 1", n)
	}
	if n := pressOneClick(t, c, d.header); n != 0 {
		t.Errorf("the repeat press reported %d stop(s), want 0", n)
	}
	if n := countRows(t, c, liveBroadStops, lead); n != 1 {
		t.Errorf("the lead holds %d stop(s) after a repeat press, want 1", n)
	}
}

// A key the catalog lacks mints the all-marketing scope, so its links name no
// purpose and the press stops all marketing rather than nothing.
func TestAnUnknownPurposeKeyStopsAllMarketing(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	c.grantMarketingByConfirmLink(t)

	d := deliverMarketing(t, c, "subject@consent.test", "no_such_purpose", true)
	assertNamesNoPurpose(t, d)
	if strings.Contains(d.page, "no_such_purpose") {
		t.Fatalf("the link repeats a key no purpose carries: %q", d.page)
	}

	if n := pressOneClick(t, c, d.header); n != 1 {
		t.Fatalf("the press reported %d stopped purpose(s), want 1", n)
	}
	manage := manageTokenFromLink(t, d.manage)
	if s, _ := purposeStateOf(t, readPreferenceView(t, c, manage), "marketing_email"); s != "withdrawn" {
		t.Fatalf("marketing_email is %q after the press, want withdrawn", s)
	}
}

// A named credential stops its own purpose, whether the press names it or not,
// and refuses a press naming another one.
func TestANamedLinkCannotBeWidened(t *testing.T) {
	c := setupConsent(t)
	c.useReviewableMarketingProfile(t)
	c.grantMarketingByConfirmLink(t)
	grantPurpose(t, c, createNewsletterPurpose(t, c))

	d := deliverMarketing(t, c, "subject@consent.test", "marketing_email", false)
	if !strings.HasSuffix(d.header, "?purpose=marketing_email") ||
		!strings.Contains(d.page, "/marketing_email?lang=en") {
		t.Fatalf("named links = %q / %q, want both to name marketing_email", d.header, d.page)
	}
	stop := tokenFromLink(t, d.page)

	if s := publicCall(t, c.AppEnv, "POST",
		"/v1/public/preferences/"+stop+"/unsubscribe?purpose=newsletter", nil, nil, nil); s != http.StatusUnprocessableEntity {
		t.Fatalf("a press naming another purpose → %d, want 422", s)
	}
	if n := pressOneClick(t, c, "/v1/public/preferences/"+stop+"/unsubscribe"); n != 1 {
		t.Fatalf("the press with no purpose reported %d stopped purpose(s), want 1", n)
	}
	view := readPreferenceView(t, c, manageTokenFromLink(t, d.manage))
	if s, _ := purposeStateOf(t, view, "marketing_email"); s != "withdrawn" {
		t.Errorf("marketing_email is %q, want withdrawn", s)
	}
	if s, _ := purposeStateOf(t, view, "newsletter"); s != "granted" {
		t.Errorf("newsletter is %q after a press on a marketing_email link, want granted", s)
	}
}
