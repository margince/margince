// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestAFormerDealLabelIsNotARecordedRequest(t *testing.T) {
	e := setupResolve(t)
	when := clocktest.Now(t).Add(-time.Hour)
	deal := e.openDeal(t, "open", true)
	// The retired kind has no current writer, but retained rows still reach the engine.
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO consent_qualifying_event
 (contact_id,kind,occurred_at,source,captured_by,source_entity_type,source_entity_id)
 VALUES (@contact,'active_deal',@when,'derived','system:fixture','deal',@deal)`,
		pgx.StrictNamedArgs{"contact": e.contact, "when": when, "deal": deal}); err != nil {
		t.Fatal(err)
	}
	if got := e.decideRequestAs(t, e.user, ids.Nil); got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("a former deal label authorized a follow-up: %s", got.ReasonCode)
	}
	if _, err := e.store.RecordQualifyingEvent(e.seatCtx(e.user), e.contact, RecordQualifyingEventInput{
		Kind: KindRequestedBySubject, Note: "Asked this company for a follow-up.", OccurredAt: when.Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if got := e.decideRequestAs(t, e.user, ids.Nil); got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("a valid request behind an obsolete label was refused: %s", got.ReasonCode)
	}
}

func TestARecordedSourceCannotAuthorizeAfterItIsRestricted(t *testing.T) {
	e := setupResolve(t)
	when := clocktest.Now(t).Add(-time.Hour)
	source := e.inboundFrom(t, "recorded-source", e.address, when)
	if _, err := e.owner.Exec(t.Context(), `UPDATE activity
 SET captured_by='connector:gmail',inbound_sender_authenticated=true WHERE id=@source`,
		pgx.StrictNamedArgs{"source": source}); err != nil {
		t.Fatal(err)
	}
	if got := e.decideRequestAs(t, e.user, ids.Nil); got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the live source did not support its request: %s", got.ReasonCode)
	}
	ctx := e.seatCtx(e.user)
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return RecordDerivedQualifyingEvent(ctx, tx, e.contact.String(), QualifyingEvent{
			Kind: "inbound_message", OccurredAt: when, SourceEntityType: "activity", SourceEntityID: source.String(),
		}, "human:"+e.user.String())
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO activity_retention_evidence
 (activity_id,basis,qualified_at,decided_by,decided_by_name,reason)
 VALUES (@source,'controller_pin',@when,@user,'Record keeper','Retain the business record')`,
		pgx.StrictNamedArgs{"source": source, "when": when, "user": e.user}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `UPDATE activity SET archived_at=now(),restricted_at=now(),
 restricted_reason='commercial_correspondence',restricted_until=now()+interval '1 year',
 retention_class='commercial_correspondence',retention_class_at=now() WHERE id=@source`,
		pgx.StrictNamedArgs{"source": source}); err != nil {
		t.Fatal(err)
	}
	if got := e.decideRequestAs(t, e.user, ids.Nil); got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("the retained copy bypassed source restriction: %s", got.ReasonCode)
	}
	var retained int
	if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM consent_qualifying_event WHERE source_entity_id=@source`,
		pgx.StrictNamedArgs{"source": source}).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatalf("retained history = %d, want one source record", retained)
	}
}

func TestAnUnscopedInquiryIsRecordedWithoutGrantingPermission(t *testing.T) {
	e := setupResolve(t)
	e.secondEntity(t)
	when := clocktest.Now(t).Add(-time.Hour)
	var source ids.UUID
	if err := e.owner.QueryRow(t.Context(), `INSERT INTO activity
 (kind,occurred_at,source,captured_by,meeting_status)
 VALUES ('meeting',@when,'public_booking','system:booking','held') RETURNING id`,
		pgx.StrictNamedArgs{"when": when}).Scan(&source); err != nil {
		t.Fatal(err)
	}
	ctx := e.seatCtx(e.user)
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return RecordSubjectInquiry(ctx, tx, e.contact.String(), QualifyingEvent{
			Kind: KindInquiry, OccurredAt: when, SourceEntityType: "activity", SourceEntityID: source.String(),
		}, "system:booking")
	}); err != nil {
		t.Fatalf("unknown company prevented recording the inquiry: %v", err)
	}
	var entities []ids.UUID
	if err := e.owner.QueryRow(t.Context(), `SELECT recorded_for_entities FROM consent_qualifying_event
 WHERE source_entity_id=@source`, pgx.StrictNamedArgs{"source": source}).Scan(&entities); err != nil {
		t.Fatal(err)
	}
	if len(entities) != 0 {
		t.Fatalf("an unscoped inquiry acquired company scope: %v", entities)
	}
	if got := e.decideRequestAs(t, e.user, ids.Nil); got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("an unscoped inquiry authorized a follow-up: %s", got.ReasonCode)
	}
}
