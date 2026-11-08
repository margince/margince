// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// One meeting face to face, recorded through each of its two doors, leaves the
// same record behind. Personal follow-up mail may go and marketing may not. The
// meeting owes no Art. 13 notice.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

const metAtTheStand = "Took their card at our stand at the trade fair."

// noticeAfter is what a door leaves on the contact's source review.
type noticeAfter struct {
	state      NoticeState
	rule       NoticeRule
	routes     []string
	resolution string
}

// unassessedSource gives the contact an unknown source under review, as a
// contact typed in by hand starts.
func (e *resolveEnv) unassessedSource(t *testing.T) ids.UUID {
	t.Helper()
	var acquisition ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
		VALUES ($1, 'unknown_legacy', 'test') RETURNING id`, e.contact).Scan(&acquisition); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
			ContactID: e.contact, AcquisitionID: acquisition, Rule: RuleUnassessed, State: NoticeSourceReview,
		})
	}); err != nil {
		t.Fatalf("opening the source review: %v", err)
	}
	var review ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM privacy_notice_case WHERE acquisition_id = $1`, acquisition).Scan(&review); err != nil {
		t.Fatal(err)
	}
	return review
}

// sendsTo reports whether the engine lets one send of this category go to the
// contact.
func (e *resolveEnv) sendsTo(t *testing.T, category commsauthz.Category) bool {
	t.Helper()
	return e.decide(t, commsauthz.Request{Context: category, Subject: "S", Body: "b"}).Verdict == commsauthz.VerdictAllow
}

func (e *resolveEnv) noticeAfter(t *testing.T, review ids.UUID) noticeAfter {
	t.Helper()
	got, err := e.store.GetNoticeCaseReview(e.officerCtx(), review)
	if err != nil {
		t.Fatalf("reading the notice case: %v", err)
	}
	if got.State == NoticeAssigned {
		got.State = NoticeOpen
	}
	out := noticeAfter{state: got.State, rule: got.Rule, routes: got.AllowedRoutes}
	if got.ResolutionNote != nil {
		out.resolution = *got.ResolutionNote
	}
	return out
}

// officerCtx is the seat with the privacy officer's grants on notice cases.
func (e *resolveEnv) officerCtx() context.Context {
	ctx := e.seatCtx(e.user)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects["privacy_request"] = principal.ObjectGrant{Read: true, Update: true}
	return principal.WithActor(ctx, actor)
}

// answerRefusedSend records the meeting as the answer to a refused follow-up,
// as the caller ctx.
func (e *resolveEnv) answerRefusedSend(ctx context.Context, t *testing.T, met time.Time) {
	t.Helper()
	refused := e.decide(t, commsauthz.Request{Context: commsauthz.CategoryRequestedFollowup, Subject: "S", Body: "b"})
	var open Review
	if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		open, err = OpenReviewTx(ctx, tx, commsauthz.DecisionSet{Decisions: []commsauthz.Decision{refused}}, ids.Nil)
		return err
	}); err != nil {
		t.Fatalf("opening the review: %v", err)
	}
	if _, err := e.store.RecordContext(ctx, open.ID, RecordContextInput{
		SubjectID: e.contact, Kind: kindInPerson, Note: metAtTheStand, OccurredAt: met,
	}); err != nil {
		t.Fatalf("recording the meeting: %v", err)
	}
}

// A meeting qualifies the same mail through either door and owes no notice.
// The source review settles as information given at the meeting, through the
// officer's writer. A rep without the notice-case grant leaves it for the
// officer.
func TestAMeetingFaceToFaceQualifiesTheSameMailThroughEitherDoor(t *testing.T) {
	now := clocktest.Now(t).UTC()
	met := now.Add(-24 * time.Hour).Truncate(time.Second)
	settled := noticeAfter{state: NoticeExemptWithReason, rule: RuleNone, routes: []string{}}
	doors := []struct {
		name   string
		record func(t *testing.T, e *resolveEnv, review ids.UUID)
		notice noticeAfter
	}{{
		name:   "a rep answering a refused send",
		record: func(t *testing.T, e *resolveEnv, _ ids.UUID) { e.answerRefusedSend(e.seatCtx(e.user), t, met) },
		notice: noticeAfter{state: NoticeSourceReview, rule: RuleUnassessed, routes: []string{}},
	}, {
		name:   "an officer answering a refused send",
		record: func(t *testing.T, e *resolveEnv, _ ids.UUID) { e.answerRefusedSend(e.officerCtx(), t, met) },
		notice: settled,
	}, {
		name: "the source review",
		record: func(t *testing.T, e *resolveEnv, review ids.UUID) {
			if _, err := e.store.AssessNoticeSource(e.officerCtx(), review, SourceAssessment{
				Kind: faceToFaceSource, MetInPerson: true, ObtainedAt: &met, Note: metAtTheStand, Now: now,
			}); err != nil {
				t.Fatalf("confirming the source: %v", err)
			}
		},
		notice: settled,
	}}
	for _, door := range doors {
		t.Run(door.name, func(t *testing.T) {
			e := setupResolve(t)
			review := e.unassessedSource(t)
			if e.sendsTo(t, commsauthz.CategoryRequestedFollowup) {
				t.Fatal("a follow-up went before the meeting was recorded")
			}
			door.record(t, e, review)
			if !e.sendsTo(t, commsauthz.CategoryRequestedFollowup) {
				t.Error("personal follow-up mail is refused after the meeting, want it allowed")
			}
			if e.sendsTo(t, commsauthz.CategoryMarketing) {
				t.Error("marketing went on the strength of a meeting, want it refused")
			}
			got := e.noticeAfter(t, review)
			if got.state != door.notice.state || got.rule != door.notice.rule || !slices.Equal(got.routes, door.notice.routes) {
				t.Errorf("source review after the meeting: %+v, want %+v", got, door.notice)
			}
			if got.state == NoticeExemptWithReason && got.resolution != faceToFaceSettled {
				t.Errorf("settled with %q, want %q", got.resolution, faceToFaceSettled)
			}
		})
	}
}

// A meeting older than the reply window qualifies no mail and owes no notice.
// The source review settles. No exchange is recorded, because the verdict
// would never read one that old.
func TestAMeetingOlderThanTheReplyWindowQualifiesNothingAndOwesNoNotice(t *testing.T) {
	e := setupResolve(t)
	review := e.unassessedSource(t)
	now := clocktest.Now(t).UTC()
	met := now.Add(-twoYears).Truncate(time.Second)
	if _, err := e.store.AssessNoticeSource(e.officerCtx(), review, SourceAssessment{
		Kind: faceToFaceSource, MetInPerson: true, ObtainedAt: &met, Note: metAtTheStand, Now: now,
	}); err != nil {
		t.Fatalf("confirming the source: %v", err)
	}
	if e.sendsTo(t, commsauthz.CategoryRequestedFollowup) {
		t.Error("a meeting two years ago qualified a follow-up, want it refused")
	}
	var events int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM consent_qualifying_event WHERE contact_id = $1`, e.contact).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 0 {
		t.Errorf("%d exchanges recorded for a meeting outside the reply window, want 0", events)
	}
	if got := e.noticeAfter(t, review); got.state != NoticeExemptWithReason || got.rule != RuleNone {
		t.Errorf("source review after the meeting: %+v, want settled with no duty", got)
	}
}
