// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A merge leaves nothing a reader of the survivor needs on the retired record.
// gates/mergereferences_test.go proves a statement moves each column; these
// prove the rows land on the survivor, through the carriers compose wires.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/erasureaccount"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The merged-away lead's hand-off, hand-set factors, send override and sent
// links follow the survivor, and an earlier merge into it now points there. On
// a factor both leads answered, the survivor's own answer stands. The sent
// link arrives expired, and each dropped factor is audited with what it held.
func TestALeadMergeCarriesTheRetiredLeadsOpenWork(t *testing.T) {
	c := setupCarry(t)
	older := c.lead(t, "Pia Lund", "pia@lund.test", "Lund AB")
	loser := c.lead(t, "Pia Lunde", "pia.lunde@lund.test", "Lund AB")
	live := c.lead(t, "Pia Lundt", "", "Lund AB")
	if _, err := c.contacts.MergeLead(c.admin, older, loser); err != nil {
		t.Fatalf("merging the older lead: %v", err)
	}
	loserID := loser.UUID
	if _, err := c.contacts.SubmitHandoff(c.admin, contacts.NewSDRHandoff{LeadID: &loserID, Note: "warm"}); err != nil {
		t.Fatalf("handing off the lead: %v", err)
	}
	setFactor := func(lead ids.LeadID, factor, band string) {
		t.Helper()
		if _, err := c.contacts.SetLeadManualSignal(c.admin, lead, contacts.SetLeadManualSignalInput{
			Factor: factor, Band: band, SignalKind: "judgement", Reason: "asked them",
		}); err != nil {
			t.Fatalf("setting %s on a lead: %v", factor, err)
		}
	}
	setFactor(loser, "budget_hint", "confirmed")
	setFactor(loser, "web_traffic", "low")
	setFactor(live, "web_traffic", "high")
	// No door records an override any more, so a row in the shape the
	// recording left is the only way to start (plantAdminOverride).
	c.e.WsExec(t, `INSERT INTO communication_override
		    (lead_id, category, reason, decided_by_level, captured_by, legal_entity_id, recorded_directly)
		VALUES ($1, 'marketing', 'they asked for the newsletter', 'admin', $2, (SELECT id FROM legal_entity WHERE is_default), true)`,
		loser, "human:"+c.e.AdminUser.String())
	// The privacy-notice mail mints this link; the row is the link it leaves.
	c.e.WsExec(t, `INSERT INTO confirm_token (lead_id, token_hash, delivered_to, expires_at, kind)
		VALUES ($1, repeat('a', 64), 'pia.lunde@lund.test', now() + interval '30 days', 'privacy_notice')`, loser)

	if _, err := c.contacts.MergeLead(c.admin, loser, live); err != nil {
		t.Fatalf("lead merge: %v", err)
	}

	for _, check := range []struct {
		what, query string
		want        int
	}{
		{"hand-offs", `SELECT count(*) FROM sdr_handoff WHERE lead_id = $1`, 1},
		{"hand-set factors", `SELECT count(*) FROM lead_manual_signal WHERE lead_id = $1`, 2},
		{"its own web_traffic answer", `SELECT count(*) FROM lead_manual_signal WHERE lead_id = $1 AND factor = 'web_traffic' AND band = 'high'`, 1},
		{"live overrides", `SELECT count(*) FROM communication_override WHERE lead_id = $1 AND revoked_at IS NULL`, 1},
		{"sent links, expired", `SELECT count(*) FROM confirm_token WHERE lead_id = $1 AND expires_at <= now()`, 1},
	} {
		if n := c.count(t, check.query, live); n != check.want {
			t.Errorf("the surviving lead holds %d %s, want %d", n, check.what, check.want)
		}
	}
	for _, table := range []string{"sdr_handoff", "lead_manual_signal", "confirm_token"} {
		if n := c.count(t, `SELECT count(*) FROM `+table+` WHERE lead_id = $1`, loser); n != 0 {
			t.Errorf("%d %s rows still name the retired lead", n, table)
		}
	}
	if n := c.count(t, `SELECT count(*) FROM lead WHERE id = $1 AND merged_into_id = $2`, older, live); n != 1 {
		t.Error("the lead merged into the retired one does not point at the survivor, so the redirect is two hops deep")
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND entity_id = $1
		AND before->'manual_signal'->>'factor' = 'web_traffic' AND after->'manual_signal' = 'null'::jsonb`, loser); n != 1 {
		t.Errorf("%d audit rows hold the dropped web_traffic factor's before-image, want 1", n)
	}
}

// A contact merge moves the signals resolved to the retired contact, through
// the signals carrier, and the privacy requests still being answered. Each
// moved request's audit names the merge. A closed request stays on the record
// it acted on.
func TestAContactMergeCarriesResolvedSignalsAndOpenRequests(t *testing.T) {
	c := setupCarry(t)
	company := c.e.SeedCompany(t, "Lund Bau", nil)
	retired := c.contactAt(t, "Jon Lund", "jon@lund.test")
	survivor := c.contactAt(t, "Jon Lunde", "jon.lunde@lund.test")
	// The resolver sets resolved_contact_id only under a recorded consent
	// grant; this row is the shape it leaves.
	signal := ids.NewV7()
	c.e.WsExec(t, `INSERT INTO signal (id, kind, summary, entity_type, entity_id, resolution_state,
		    resolved_company_id, resolved_contact_id, status, source, captured_by)
		VALUES ($1, 'buying_intent', 'they read the pricing page', 'company', $2, 'resolved', $2, $3, 'open', 'manual', 'human:x')`,
		signal, company, retired)
	open := c.erasureRequest(t, retired)
	closed := ids.NewV7()
	c.e.WsExec(t, `INSERT INTO data_subject_request (id, kind, status, subject_ref, contact_id, due_at, resolution)
		VALUES ($1, 'access', 'fulfilled', 'jon@lund.test', $2, now() + interval '30 days', 'sent the export')`, closed, retired)

	c.merge(t, retired, survivor)

	if n := c.count(t, `SELECT count(*) FROM signal WHERE id = $1 AND resolved_contact_id = $2`, signal, survivor); n != 1 {
		t.Error("the signal resolved to the retired contact does not resolve to the survivor")
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1`, signal); n != 1 {
		t.Errorf("%d audit rows record the moved signal, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM data_subject_request WHERE id = $1 AND contact_id = $2`, open, survivor); n != 1 {
		t.Error("the open request does not follow the survivor")
	}
	if n := c.count(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'data_subject_request' AND entity_id = $1
		AND after->>'merged_from' = $2::text AND after->'filed_for_addresses' ? 'jon@lund.test'`, open, retired); n != 1 {
		t.Errorf("%d audit rows on the moved request name the merge and the address it was filed for, want 1", n)
	}
	if n := c.count(t, `SELECT count(*) FROM data_subject_request WHERE id = $1 AND contact_id = $2`, closed, retired); n != 1 {
		t.Error("the fulfilled request left the record it acted on")
	}
}

// An erasure request a merge moved is fulfilled against the survivor, which
// now holds the person's data.
func TestAnErasureRequestMovedByAMergeIsFulfilledOnTheSurvivor(t *testing.T) {
	c := setupCarry(t)
	retired := c.contactAt(t, "Eva Lund", "eva@lund.test")
	survivor := c.contactAt(t, "Eva Lunde", "eva.lunde@lund.test")
	request := c.erasureRequest(t, retired)
	c.merge(t, retired, survivor)

	eraser := ComposedEraser(c.e.Pool, nil)
	var erased ids.UUID
	erase := func(ctx context.Context, contactID ids.UUID, reason string) (erasureaccount.Account, error) {
		erased = contactID
		return eraser.EraseContactCounted(ctx, contactID, reason)
	}
	if _, err := c.consent.FulfilErasure(c.admin, request, consent.UpdateDSRInput{
		Status: new("fulfilled"), Proof: consent.ProofStatement{IdentityCheck: new("id_document")},
	}, erase); err != nil {
		t.Fatalf("fulfilling the moved erasure: %v", err)
	}
	if erased != survivor.UUID {
		t.Errorf("the moved erasure erased %s, want the survivor %s", erased, survivor)
	}
}

// erasureRequest files an erasure request through the queue and links it to
// the contact, as a request filed through the contact's own confirm link is.
// That door needs a mailed link, so the row is given the link it leaves.
func (c *carryEnv) erasureRequest(t *testing.T, contact ids.ContactID) ids.UUID {
	t.Helper()
	row, err := c.consent.CreateDSR(c.admin, consent.CreateDSRInput{Kind: "erasure", SubjectRef: contact.String()})
	if err != nil {
		t.Fatalf("filing the erasure request: %v", err)
	}
	c.e.WsExec(t, `UPDATE data_subject_request SET contact_id = $2 WHERE id = $1`, row.ID, contact)
	return row.ID
}
