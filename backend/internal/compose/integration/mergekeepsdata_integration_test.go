// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A merge leaves nothing a reader of the survivor needs on the retired record.
// gates/mergereferences_test.go proves a statement moves each column; these
// prove the rows land on the survivor.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The merged-away lead's hand-off, hand-set factors, send override and sent
// links follow the survivor, and an earlier merge into it now points there. On
// a factor both leads answered, the survivor's own answer stands.
func TestALeadMergeCarriesTheRetiredLeadsOpenWork(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	older := seedLead(t, e, "Pia Lund", "pia@lund.test", nil)
	loser := seedLead(t, e, "Pia Lunde", "pia.lunde@lund.test", nil)
	live := seedLead(t, e, "Pia Lundt", "", nil)
	if _, err := e.Contacts.MergeLead(admin, older, loser); err != nil {
		t.Fatalf("merging the older lead: %v", err)
	}
	loserID := loser.UUID
	if _, err := e.Contacts.SubmitHandoff(admin, contacts.NewSDRHandoff{LeadID: &loserID, Note: "warm"}); err != nil {
		t.Fatalf("handing off the lead: %v", err)
	}
	setFactor := func(lead ids.LeadID, factor, band string) {
		t.Helper()
		if _, err := e.Contacts.SetLeadManualSignal(admin, lead, contacts.SetLeadManualSignalInput{
			Factor: factor, Band: band, SignalKind: "judgement", Reason: "asked them",
		}); err != nil {
			t.Fatalf("setting %s on a lead: %v", factor, err)
		}
	}
	setFactor(loser, "budget_hint", "confirmed")
	setFactor(loser, "web_traffic", "low")
	setFactor(live, "web_traffic", "high")
	// No door records an override any more, so a row in the shape the
	// recording left is the only way to start (mergecarriesstops_integration_test.go).
	e.WsExec(t, `INSERT INTO communication_override
		    (lead_id, category, reason, decided_by_level, captured_by, legal_entity_id, recorded_directly)
		VALUES ($1, 'marketing', 'they asked for the newsletter', 'admin', $2, (SELECT id FROM legal_entity WHERE is_default), true)`,
		loser, "human:"+e.AdminUser.String())
	// The privacy-notice mail mints this link; the row is the link it leaves.
	e.WsExec(t, `INSERT INTO confirm_token (lead_id, token_hash, delivered_to, expires_at, kind)
		VALUES ($1, repeat('a', 64), 'pia.lunde@lund.test', now() + interval '30 days', 'privacy_notice')`, loser)

	if _, err := e.Contacts.MergeLead(admin, loser, live); err != nil {
		t.Fatalf("lead merge: %v", err)
	}

	for _, c := range []struct {
		what, query string
		want        int
	}{
		{"hand-offs", `SELECT count(*) FROM sdr_handoff WHERE lead_id = $1`, 1},
		{"hand-set factors", `SELECT count(*) FROM lead_manual_signal WHERE lead_id = $1`, 2},
		{"its own web_traffic answer", `SELECT count(*) FROM lead_manual_signal WHERE lead_id = $1 AND factor = 'web_traffic' AND band = 'high'`, 1},
		{"live overrides", `SELECT count(*) FROM communication_override WHERE lead_id = $1 AND revoked_at IS NULL`, 1},
		{"sent links", `SELECT count(*) FROM confirm_token WHERE lead_id = $1`, 1},
	} {
		if n := e.WsCount(t, c.query, live); n != c.want {
			t.Errorf("the surviving lead holds %d %s, want %d", n, c.what, c.want)
		}
	}
	for _, table := range []string{"sdr_handoff", "lead_manual_signal", "confirm_token"} {
		if n := e.WsCount(t, `SELECT count(*) FROM `+table+` WHERE lead_id = $1`, loser); n != 0 {
			t.Errorf("%d %s rows still name the retired lead", n, table)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND merged_into_id = $2`, older, live); n != 1 {
		t.Error("the lead merged into the retired one does not point at the survivor, so the redirect is two hops deep")
	}
}

// A contact merge moves the signals resolved to the retired contact and the
// privacy requests still being answered. A closed request stays on the record
// it acted on.
func TestAContactMergeCarriesResolvedSignalsAndOpenRequests(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Lund Bau", nil)
	retired, survivor := e.SeedContact(t, "Jon Lund", nil), e.SeedContact(t, "Jon Lunde", nil)
	// The resolver sets resolved_contact_id only under a recorded consent
	// grant; this row is the shape it leaves.
	e.WsExec(t, `INSERT INTO signal (kind, summary, entity_type, entity_id, resolution_state,
		    resolved_company_id, resolved_contact_id, status, source, captured_by)
		VALUES ('buying_intent', 'they read the pricing page', 'company', $1, 'resolved', $1, $2, 'open', 'manual', 'human:x')`,
		company, retired)
	open, closed := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO data_subject_request (id, kind, status, subject_ref, contact_id, due_at)
		VALUES ($1, 'access', 'open', 'jon@lund.test', $2, now() + interval '30 days')`, open, retired)
	e.WsExec(t, `INSERT INTO data_subject_request (id, kind, status, subject_ref, contact_id, due_at, resolution)
		VALUES ($1, 'access', 'fulfilled', 'jon@lund.test', $2, now() + interval '30 days', 'sent the export')`, closed, retired)

	if _, err := e.Contacts.MergeContact(e.Admin(), ContactIDOf(retired), ContactIDOf(survivor), nil); err != nil {
		t.Fatalf("contact merge: %v", err)
	}

	if n := e.WsCount(t, `SELECT count(*) FROM signal WHERE resolved_contact_id = $1`, survivor); n != 1 {
		t.Errorf("%d signals resolve to the survivor, want the 1 that resolved to the retired contact", n)
	}
	for _, c := range []struct {
		request, contact ids.UUID
		what             string
	}{
		{open, survivor, "the open request follows the survivor"},
		{closed, retired, "the fulfilled request stays on the record it acted on"},
	} {
		if n := e.WsCount(t, `SELECT count(*) FROM data_subject_request WHERE id = $1 AND contact_id = $2`, c.request, c.contact); n != 1 {
			t.Errorf("%s, but it names another contact", c.what)
		}
	}
}
