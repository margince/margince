// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// A revoke reaches the whole carry chain — and both halves of "reaches" are
// separately defeatable, so each gets its own case here.
//
// The WRITE half is already held elsewhere (the merge suite's
// TestRevokingAPreMergeOverrideHandleStopsTheSendOnTheSurvivor drives a real
// send decision across a merge). What that case cannot see is the two ways the
// walk still fails OPEN, because neither is visible to a test that runs the
// writers one after another:
//
//   - a chain that GROWS while the walk runs, leaving a live descendant under
//     a revoked ancestor, and
//   - a row revoked in the table that no subscriber is ever told about.
//
// Both are failures of what the revoke REACHES rather than of what it decides,
// which is why they sit beside overridechain.go and not with the authority
// cases in override_integration_test.go.

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// TestRevokingACarriedVouchAnnouncesEveryRowItTookBack is the subscriber's half.
//
// Each carried copy ships its OWN consent.override_recorded naming the new row's
// id, on the survivor's stream (overridecarry.go turns on that being so). A
// lifted event naming only the id the caller typed therefore leaves the
// survivor's stream holding a vouch that is revoked in the table and live to
// everyone reading the bus — the send stops, and every consumer that mirrored
// the record goes on believing a rep is still standing behind it.
func TestRevokingACarriedVouchAnnouncesEveryRowItTookBack(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := seedOverrideContact(t, e, "Carry Survivor")

	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	carryOverrides(t, e, e.contact, survivor)
	copied := liveOverrideID(t, e, survivor)

	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: recorded, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("revoking the override: %v", err)
	}

	lifted := liftedOverrideStreams(t, e)
	if got := lifted[recorded]; got != e.contact.UUID {
		t.Errorf("the source override was announced on stream %s, want the contact that recorded it, %s",
			got, e.contact.UUID)
	}
	if got := lifted[copied]; got != survivor.UUID {
		t.Errorf("the carried copy was announced on stream %s, want the survivor's, %s — the copy was "+
			"announced as recorded on that stream and nothing there ever takes it back, so a consumer "+
			"holds a vouch the table revoked", got, survivor.UUID)
	}
}

// TestAMergeCannotCarryAVouchPastTheRevokeTakingItBack is the write half's
// concurrency, and the reason a subject lock is not enough.
//
// The walk's recursive term reads its statement's snapshot. A merge OUT of the
// survivor locks two subjects the revoker never names — the chain outlives the
// record it started on — so without a key both writers derive, that merge can
// commit a further copy just after the snapshot is taken and leave it live
// beneath a revoked ancestor. The vouch then goes on allowing sends to a
// contact its author never heard of.
//
// Arranged rather than raced: the second carry is held open in its own
// transaction and the revoke is watched reaching its lock, so the interleaving
// is observed instead of hoped for. Defeated in both directions — remove the
// family lock and the revoke never queues, which the wait reports; leave the
// walk short and the third record keeps a live row, which the count reports.
func TestAMergeCannotCarryAVouchPastTheRevokeTakingItBack(t *testing.T) {
	e := setupChannelConsent(t)
	middle := seedOverrideContact(t, e, "Chain Middle")
	last := seedOverrideContact(t, e, "Chain Last")

	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	carryOverrides(t, e, e.contact, middle)

	// The second merge's carry, held open on its own connection: it has copied
	// the vouch onto `last` and not yet committed, which is exactly the window
	// the revoke's snapshot would miss.
	holding, release := carryHeldOpen(t, e, middle, last)
	revoked := make(chan error, 1)
	go func() {
		revoked <- e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
			ContactID: e.contact, OverrideID: recorded, Reason: "the buyer changed their mind",
		})
	}()

	waitUntilBlockedByPID(t, e.owner, holding, 1)
	release()

	if err := <-revoked; err != nil {
		t.Fatalf("revoking once the carry committed: %v", err)
	}
	for _, subject := range []ids.ContactID{e.contact, middle, last} {
		if n := liveOverridesOn(t, e, subject); n != 0 {
			t.Errorf("contact %s holds %d live override(s) after the vouch was taken back, want 0: a "+
				"merge running against the revoke left a copy standing, and mail goes out on a vouch "+
				"somebody revoked", subject.UUID, n)
		}
	}
}

// TestRevokingACarriedCopyTakesBackItsOriginalToo is the walk's other
// direction. The family lock is keyed on the chain's ROOT, so the set it
// serialises is the whole chain whichever member the caller names — and the
// write has to cover the same set, or a revoke through the survivor's id
// leaves the original standing on the retired record, where the SAR export
// and anything that un-merges still finds a live vouch.
func TestRevokingACarriedCopyTakesBackItsOriginalToo(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := seedOverrideContact(t, e, "Copy Survivor")

	if _, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	}); err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	carryOverrides(t, e, e.contact, survivor)
	copied := liveOverrideID(t, e, survivor)

	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: survivor, OverrideID: copied, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("revoking the carried copy: %v", err)
	}

	if n := liveOverridesOn(t, e, e.contact); n != 0 {
		t.Errorf("the retired contact still holds %d live override(s) after its copy was revoked, "+
			"want 0: the walk started at the copy and never reached the original", n)
	}
	if n := liveOverridesOn(t, e, survivor); n != 0 {
		t.Errorf("the survivor still holds %d live override(s), want 0", n)
	}
}

// seedOverrideContact plants a contact for a chain to run through.
func seedOverrideContact(t *testing.T, e *channelConsentEnv, name string) ids.ContactID {
	t.Helper()
	id := ids.New[ids.ContactKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact (id, full_name, source, captured_by, visibility, owner_id)
		VALUES ($1, $2, 'test', 'human:x', 'workspace', $3)`, id, name, e.user); err != nil {
		t.Fatalf("seeding %s: %v", name, err)
	}
	return id
}

// carryOverrides runs the merge's own carry, through the production writer the
// contacts→consent seam calls, so what the chain looks like afterwards is what
// a real merge leaves.
func carryOverrides(t *testing.T, e *channelConsentEnv, from, to ids.ContactID) {
	t.Helper()
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.CarryOverridesTx(e.ctx, tx,
			commsauthz.ContactStopSubject(from), commsauthz.ContactStopSubject(to))
	}); err != nil {
		t.Fatalf("carrying the overrides onto the survivor: %v", err)
	}
}

// carryHeldOpen runs the same carry and stops before COMMIT, answering the
// backend pid it is holding its locks on and the release that lets it finish.
//
// The pid is read inside the transaction because the carry runs on a pooled
// connection the test never names — and pid is the whole question the wait asks,
// since "somebody is blocked" would return for a waiter belonging to another
// case in the same database.
func carryHeldOpen(t *testing.T, e *channelConsentEnv, from, to ids.ContactID) (int, func()) {
	t.Helper()
	holding := make(chan int, 1)
	finish := make(chan struct{})
	committed := make(chan error, 1)
	go func() {
		committed <- e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			if err := e.store.CarryOverridesTx(e.ctx, tx,
				commsauthz.ContactStopSubject(from), commsauthz.ContactStopSubject(to)); err != nil {
				return err
			}
			var pid int
			if err := tx.QueryRow(e.ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return err
			}
			holding <- pid
			<-finish
			return nil
		})
	}()
	var released bool
	release := func() {
		if released {
			return
		}
		released = true
		close(finish)
		if err := <-committed; err != nil {
			t.Errorf("committing the held-open carry: %v", err)
		}
	}
	t.Cleanup(release)
	select {
	case pid := <-holding:
		return pid, release
	case err := <-committed:
		// The carry failed before it could hold anything; the error is the
		// finding, and release must not wait on a channel already drained.
		released = true
		t.Fatalf("the held-open carry failed before holding its locks: %v", err)
		return 0, release
	}
}

// liveOverrideID reads back the one live override a subject holds.
func liveOverrideID(t *testing.T, e *channelConsentEnv, contact ids.ContactID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT id FROM communication_override
		 WHERE contact_id = $1 AND revoked_at IS NULL`, contact).Scan(&id); err != nil {
		t.Fatalf("reading the live override: %v", err)
	}
	return id
}

// liveOverridesOn counts what is still standing for a subject.
func liveOverridesOn(t *testing.T, e *channelConsentEnv, contact ids.ContactID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM communication_override
		 WHERE contact_id = $1 AND revoked_at IS NULL`, contact).Scan(&n); err != nil {
		t.Fatalf("counting live overrides: %v", err)
	}
	return n
}

// liftedOverrideStreams reads every staged consent.override_lifted back as
// "which override, announced on whose stream".
//
// Read off event_outbox rather than asserted against the call, because the row
// a consumer will actually receive is the thing under test — the same reason
// lastRecordedOverridePayload reads it there.
func liftedOverrideStreams(t *testing.T, e *channelConsentEnv) map[ids.UUID]ids.UUID {
	t.Helper()
	rows, err := e.owner.Query(context.Background(), `
		SELECT (envelope->'entity'->>'id')::uuid, envelope->'payload'
		  FROM event_outbox
		 WHERE envelope->>'type' = 'consent.override_lifted'`)
	if err != nil {
		t.Fatalf("reading the staged lifted events: %v", err)
	}
	defer rows.Close()
	streams := map[ids.UUID]ids.UUID{}
	for rows.Next() {
		var entity ids.UUID
		var raw []byte
		if err := rows.Scan(&entity, &raw); err != nil {
			t.Fatalf("reading a staged lifted event: %v", err)
		}
		var payload crmcontracts.PublicEventConsentOverrideLifted
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("decoding the lifted payload: %v", err)
		}
		streams[ids.UUID(payload.OverrideId)] = entity
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the staged lifted events: %v", err)
	}
	return streams
}

// TestRevokingAVouchAuditsEveryContactTheChainReached is the auditor's half,
// beside the subscriber's above. A carry writes "overrides carried" into the
// survivor's history; a revoke that audits only the record the caller named
// leaves that history ending there, with the copy revoked in the table and
// nothing on the survivor saying so. One audit row per row taken back, on the
// contact that row belongs to, is what keeps each record's history complete.
func TestRevokingAVouchAuditsEveryContactTheChainReached(t *testing.T) {
	e := setupChannelConsent(t)
	survivor := seedOverrideContact(t, e, "Audit Survivor")

	recorded, err := e.store.Allow(e.ctx, AllowInput{
		ContactID: e.contact, Category: "marketing", Reason: "they asked us at the trade fair",
	})
	if err != nil {
		t.Fatalf("recording the override: %v", err)
	}
	carryOverrides(t, e, e.contact, survivor)
	copied := liveOverrideID(t, e, survivor)

	if err := e.store.RevokeOverride(e.ctx, RevokeOverrideInput{
		ContactID: e.contact, OverrideID: recorded, Reason: "the buyer changed their mind",
	}); err != nil {
		t.Fatalf("revoking the override: %v", err)
	}

	for _, want := range []struct {
		contact ids.ContactID
		row     ids.UUID
	}{{e.contact, recorded}, {survivor, copied}} {
		if n := revokeAuditRows(t, e, want.contact, want.row); n != 1 {
			t.Errorf("contact %s carries %d audit row(s) naming revoked override %s, want 1",
				want.contact.UUID, n, want.row)
		}
	}
}

// revokeAuditRows counts the audit entries on one contact that name one
// revoked override — the row an auditor opening that contact's history reads.
func revokeAuditRows(t *testing.T, e *channelConsentEnv, contact ids.ContactID, row ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'update'
		   AND after->>'revoked_override' = $2`, contact, row.String()).Scan(&n); err != nil {
		t.Fatalf("counting the revoke's audit rows: %v", err)
	}
	return n
}
