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

func TestAPrivateActivitySupportsOnlyItsAudience(t *testing.T) {
	for _, kind := range []string{"inbound", "meeting", "inquiry"} {
		t.Run(kind, func(t *testing.T) {
			e := setupResolve(t)
			when := clocktest.Now(t).Add(-time.Hour)
			source := e.inboundFrom(t, "private-evidence", e.address, when)
			if _, err := e.owner.Exec(t.Context(), `UPDATE activity SET audience='participants',
 captured_by=@actor, inbound_sender_authenticated=true WHERE id=@source`, pgx.StrictNamedArgs{"source": source, "actor": "connector:gmail:" + e.user.String()}); err != nil {
				t.Fatal(err)
			}
			if _, err := e.owner.Exec(t.Context(), `INSERT INTO activity_participant (activity_id,user_id,role)
 VALUES (@source,@user,'to')`, pgx.StrictNamedArgs{"source": source, "user": e.user}); err != nil {
				t.Fatal(err)
			}
			if kind == "meeting" {
				if _, err := e.owner.Exec(t.Context(), `UPDATE activity SET kind='meeting',direction=NULL,counterparty_email=NULL,
 meeting_status='held' WHERE id=@source`, pgx.StrictNamedArgs{"source": source}); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "inquiry" {
				ctx := e.seatCtx(e.user)
				if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
					return RecordSubjectInquiry(ctx, tx, e.contact.String(), QualifyingEvent{
						Kind: KindInquiry, OccurredAt: when,
						SourceEntityType: "activity", SourceEntityID: source.String(),
					}, "connector:gmail")
				}); err != nil {
					t.Fatal(err)
				}
			}
			colleague := e.colleague(t, ids.Nil)
			if got := e.decideRequestAs(t, e.user, ids.Nil); got.Verdict != commsauthz.VerdictAllow {
				t.Fatalf("the audience member was refused: %s", got.ReasonCode)
			}
			if got := e.decideRequestAs(t, colleague, ids.Nil); got.Verdict == commsauthz.VerdictAllow {
				t.Fatal("private evidence authorized a colleague outside its audience")
			}
		})
	}
}

func TestAnsweringAnOldReviewKeepsTheOriginalCompany(t *testing.T) {
	e := setupResolve(t)
	branch := e.secondEntity(t)
	sourceReview := e.unassessedSource(t)
	refused := e.decideRequestAs(t, e.user, ids.Nil)
	var review Review
	ctx := e.officerCtx()
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		review, err = OpenReviewTx(ctx, tx, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{refused}}, ids.Nil)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(t.Context(), `INSERT INTO legal_entity_member (user_id,legal_entity_id,assigned_by)
 VALUES (@user,@branch,'human:x')`, pgx.StrictNamedArgs{"user": e.user, "branch": branch}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.RecordContext(e.officerCtx(), review.ID, RecordContextInput{
		SubjectID: e.contact, Kind: kindInPerson, Note: metAtTheStand, OccurredAt: clocktest.Now(t).Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	var count, wrongCompany int
	if err := e.owner.QueryRow(t.Context(), `SELECT count(*),count(*) FILTER (WHERE @branch=ANY(recorded_for_entities))
 FROM consent_qualifying_event WHERE contact_id=@contact`, pgx.StrictNamedArgs{"branch": branch, "contact": e.contact}).Scan(&count, &wrongCompany); err != nil {
		t.Fatal(err)
	}
	if count != 1 || wrongCompany != 0 {
		t.Fatalf("recorded meetings=%d, meetings for the responder's new company=%d", count, wrongCompany)
	}
	if got := e.noticeAfter(t, sourceReview); got.state != NoticeExemptWithReason {
		t.Fatalf("source review state=%s", got.state)
	}
}
