// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A product-built unsubscribe link, pressed over the public edge, stops what
// its credential covers and answers 200. Driven from the HTTP send, through
// the header the recipient's mailbox provider reads, to the next send's gate.

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// sentHeader sends one message under purpose and returns its List-Unsubscribe URL.
func sentHeader(t *testing.T, c *consentEnv, purpose string) *url.URL {
	t.Helper()
	if status, _ := sendMarketing(t, c.AppEnv, c.activityID, purpose, "", ""); status != http.StatusAccepted {
		t.Fatalf("send under %q → %d, want 202", purpose, status)
	}
	var header string
	if err := c.Owner.QueryRow(context.Background(),
		`SELECT coalesce(list_unsubscribe, '') FROM comms_outbound ORDER BY id DESC LIMIT 1`).Scan(&header); err != nil {
		t.Fatalf("reading the staged header: %v", err)
	}
	link, err := url.Parse(strings.Trim(header, "<>"))
	if err != nil || header == "" {
		t.Fatalf("the send under %q carries no List-Unsubscribe URL (%q, %v)", purpose, header, err)
	}
	return link
}

// pressHeader POSTs the header URL as a mailbox provider does, with an
// optional replacement query, and returns the status.
func pressHeader(t *testing.T, c *consentEnv, link *url.URL, query string) int {
	t.Helper()
	if query == "" {
		query = link.RawQuery
	}
	path := link.Path
	if query != "" {
		path += "?" + query
	}
	return publicCall(t, c.AppEnv, "POST", path, nil, nil, nil)
}

func grantCreatedPurpose(t *testing.T, c *consentEnv, key string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	if status := c.Call(t, "POST", "/v1/consent-purposes", AnyMap{
		"key": key, "label": key, "requires_double_opt_in": false,
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create purpose %s → %d", key, status)
	}
	grantPurpose(t, c, created.ID)
}

// A correspondence send carries the all-marketing credential: its header names
// no purpose, and the press stops marketing, never correspondence.
func TestACorrespondenceSendsHeaderStopsMarketingNotCorrespondence(t *testing.T) {
	c := setupConsent(t)
	grantCreatedPurpose(t, c, "newsletter")
	grantPurpose(t, c, c.purposes["business_correspondence"])

	link := sentHeader(t, c, "business_correspondence")
	if link.Query().Has("purpose") {
		t.Fatalf("the header %q names a purpose its credential may not stop", link)
	}
	if status := pressHeader(t, c, link, ""); status != http.StatusOK {
		t.Fatalf("pressing the product's own header → %d, want 200", status)
	}
	if s, code := c.send(t, "newsletter"); s != http.StatusConflict || code != "consent_not_granted" {
		t.Errorf("the newsletter after the press → %d %q, want 409 consent_not_granted", s, code)
	}
	if s, code := c.send(t, "business_correspondence"); s != http.StatusAccepted {
		t.Errorf("correspondence after the press → %d %q, want 202: an unsubscribe must never end replies", s, code)
	}
}

// A named link cannot be widened: a press naming another purpose is refused,
// and its own press leaves the other purpose sending.
func TestANamedHeaderCannotBeWidened(t *testing.T) {
	c := setupConsent(t)
	grantCreatedPurpose(t, c, "newsletter")
	grantCreatedPurpose(t, c, "offers")

	link := sentHeader(t, c, "newsletter")
	if got := link.Query().Get("purpose"); got != "newsletter" {
		t.Fatalf("the header names %q, want newsletter", got)
	}
	if status := pressHeader(t, c, link, "purpose=offers"); status == http.StatusOK {
		t.Fatal("a newsletter link stopped the offers")
	}
	if status := pressHeader(t, c, link, ""); status != http.StatusOK {
		t.Fatalf("the link's own press → %d, want 200", status)
	}
	if s, code := c.send(t, "newsletter"); s != http.StatusConflict || code != "consent_not_granted" {
		t.Errorf("the newsletter after its own press → %d %q, want 409", s, code)
	}
	if s, code := c.send(t, "offers"); s != http.StatusAccepted {
		t.Errorf("the offers after the newsletter press → %d %q, want 202", s, code)
	}
}
