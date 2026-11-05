// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A held meeting proves the buyer attended only when an attendee is outside
// by the rule capture used to keep the meeting. The domains come from the
// workspace tables capture reads.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldMeeting writes a held meeting on the deal as calendar capture stores it:
// the syncing seat as its From participant, then the attendees.
func (f *authorshipFixture) heldMeeting(seat string, attendees ...string) ids.UUID {
	f.t.Helper()
	var id ids.UUID
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `INSERT INTO activity (kind, subject, meeting_status, occurred_at, source, captured_by)
			VALUES ('meeting', 'Review', 'held', $1, 'gcal:seed', 'connector:gcal') RETURNING id`,
			clocktest.Now(f.t).Add(-time.Hour)).Scan(&id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, user_id, address, address_domain_folded, role)
			VALUES ($1, $2, $3, $4, 'from')`, id, f.AdminUser, seat, owndomain.StoredOfAddress(seat)); err != nil {
			return err
		}
		for _, a := range attendees {
			if _, err := tx.Exec(ctx, `INSERT INTO activity_participant (activity_id, address, address_domain_folded, role)
				VALUES ($1, $2, $3, 'attendee')`, id, a, owndomain.StoredOfAddress(a)); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, f.dealID.UUID)
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return id
}

// buyers is how many attendees the evidence writers count as the buyer.
func (f *authorshipFixture) buyers(meeting ids.UUID) int {
	f.t.Helper()
	var n int
	if err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		facts, err := deals.ReadActivityAuthorship(f.Admin(), tx, meeting, ownDomainReader{store: f.own})
		n = facts.BuyerParticipants
		return err
	}); err != nil {
		f.t.Fatal(err)
	}
	return n
}

func (f *authorshipFixture) wantBuyers(name string, meeting ids.UUID, want int) {
	f.t.Helper()
	if got := f.buyers(meeting); got != want {
		f.t.Errorf("%s: %d attendees counted as the buyer, want %d", name, got, want)
	}
	proof := deals.MeetingWasHeld("held", false, f.buyers(meeting))
	if proof.Held != (want > 0) {
		f.t.Errorf("%s: the meeting proves it was held = %v, want %v", name, proof.Held, want > 0)
	}
}

func TestAMeetingWithNoConfirmedOwnDomainCountsNoColleagueAsTheBuyer(t *testing.T) {
	f := newAuthorshipFixture(t)
	if domains, err := f.own.Colleagues(f.Admin()); err != nil || !domains.Empty() {
		t.Fatalf("the workspace starts with confirmed own domains %v (err %v); this test needs none", domains.Domains(), err)
	}
	f.wantBuyers("colleagues only", f.heldMeeting("rep@ourco.test", "colleague@ourco.test"), 0)
	f.wantBuyers("a real buyer", f.heldMeeting("rep@ourco.test", "colleague@ourco.test", "ines@customer.test"), 1)
	f.wantBuyers("a consumer seat domain", f.heldMeeting("rep@gmail.com", "friend@gmail.com"), 1)
	f.wantBuyers("an unknown guest", f.heldMeeting("rep@ourco.test", "guest"), 0)
}

func TestAMeetingOnACandidateOwnDomainCountsNoColleagueAsTheBuyer(t *testing.T) {
	f := newAuthorshipFixture(t)
	f.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('ourco.test', 'admin', true)`)
	f.WsExec(t, `INSERT INTO workspace_email_domain (domain) VALUES ('labs.test')`)
	f.wantBuyers("a candidate seat domain", f.heldMeeting("rep@labs.test", "dev@labs.test"), 0)
	f.wantBuyers("a confirmed domain", f.heldMeeting("rep@labs.test", "colleague@ourco.test"), 0)
	f.wantBuyers("a candidate domain the seat is not on", f.heldMeeting("rep@ourco.test", "dev@labs.test"), 1)
	f.wantBuyers("a real buyer", f.heldMeeting("rep@ourco.test", "ines@customer.test"), 1)
}

// heldCriterion adds a required event_held criterion to the fixture's stage.
func (f *authorshipFixture) heldCriterion() {
	f.t.Helper()
	required := true
	c, err := f.Deals.CreateStageExitCriterion(f.Admin(), deals.CreateCriterionInput{
		StageID: f.stageID, Key: "held", Label: "held", Kind: string(deals.CriterionEventHeld), Required: &required,
	})
	if err != nil {
		f.t.Fatal(err)
	}
	f.crit["held"] = ids.From[ids.ExitCriterionKind](ids.UUID(c.Id))
}

// heldClaim records that the meeting was held, as the given extractor.
func (f *authorshipFixture) heldClaim(meeting ids.UUID, extractedBy string) ids.UUID {
	f.t.Helper()
	ev, err := f.Deals.RecordStageEvidence(f.Admin(), deals.EvidenceInput{
		DealID: f.dealID, CriterionID: f.crit["held"], SourceType: deals.SourceActivity, SourceID: meeting,
		AuthorSide: deals.AuthorUnknown, Commitment: deals.CommitmentAgreed, Met: true,
		ObservedAt: clocktest.Now(f.t).Add(-time.Hour), ExtractedBy: extractedBy,
	})
	if err != nil {
		f.t.Fatalf("recording a held claim: %v", err)
	}
	return ids.UUID(ev.Id)
}

func (f *authorshipFixture) met(evidence ids.UUID) string {
	f.t.Helper()
	return f.WsScalar(f.t, `SELECT met::text FROM deal_stage_evidence WHERE id = $1`, evidence)
}

// A machine claim written under the old buyer rule stands until the pass
// judges the meeting again. A claim the meeting still makes, a refuted one and
// a model's reading are left as they are.
func TestAStoredMeetingClaimTheMeetingNoLongerMakesIsWithdrawn(t *testing.T) {
	f := newAuthorshipFixture(t)
	f.heldCriterion()
	stale := f.heldClaim(f.heldMeeting("rep@ourco.test", "colleague@ourco.test"), deals.ExtractedByDeterministic)
	genuine := f.heldClaim(f.heldMeeting("rep@ourco.test", "ines@customer.test"), deals.ExtractedByDeterministic)
	refuted := f.heldClaim(f.heldMeeting("rep@ourco.test", "pat@ourco.test"), deals.ExtractedByDeterministic)
	if _, err := f.Deals.RefuteStageEvidence(f.Admin(), f.dealID, refuted); err != nil {
		t.Fatal(err)
	}
	read := f.heldClaim(f.heldMeeting("rep@ourco.test", "sam@ourco.test"), stageEvidenceExtractedBy)

	f.runPass()
	for name, c := range map[string]struct {
		id   ids.UUID
		want string
	}{"stale": {stale, "false"}, "genuine": {genuine, "true"}, "refuted": {refuted, "true"}, "model reading": {read, "true"}} {
		if got := f.met(c.id); got != c.want {
			t.Errorf("the %s claim is met = %s after the pass, want %s", name, got, c.want)
		}
	}
	audits := `SELECT count(*) FROM audit_log WHERE entity_type = 'deal_stage_evidence' AND entity_id = $1
		AND evidence->>'rejudged_against' = 'meeting_parties'`
	if n := f.WsCount(t, audits, stale); n != 1 {
		t.Errorf("%d audit rows name the withdrawal, want 1", n)
	}
	f.runPass()
	if n := f.WsCount(t, audits, stale); n != 1 {
		t.Errorf("a second pass wrote %d audit rows for the withdrawal, want still 1", n)
	}
}

// The stage-move check judges the meeting live, so a stale claim settles
// nothing even before the pass has run.
func TestACardOnAStaleMeetingClaimIsRefusedBeforeThePass(t *testing.T) {
	f := newAuthorshipFixture(t)
	f.heldCriterion()
	stale := f.heldClaim(f.heldMeeting("rep@ourco.test", "colleague@ourco.test"), deals.ExtractedByDeterministic)
	checked := deals.NewStore(f.DB(), DealsInstallation()).WithOwnDomains(ownDomainReader{store: f.own})
	err := database.WithWorkspaceTx(f.Admin(), f.Pool, func(tx pgx.Tx) error {
		return checked.ValidateProgressionCriteriaTx(f.Admin(), tx, f.cardFor("held"))
	})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("a card resting on a meeting only colleagues attended stands: %v", err)
	}
	if got := f.met(stale); got != "true" {
		t.Errorf("the check changed the stored claim to met = %s; only the pass writes", got)
	}
}

// The pass sets a machine claim to what the meeting proves now, both ways. A
// domain added and removed again, or a status corrected, brings it back.
func TestAWithdrawnMeetingClaimComesBackWhenTheMeetingProvesItAgain(t *testing.T) {
	f := newAuthorshipFixture(t)
	f.heldCriterion()
	byDomain := f.heldClaim(f.heldMeeting("rep@ourco.test", "ines@buyer-corp.test"), deals.ExtractedByDeterministic)
	canceled := f.heldMeeting("rep@ourco.test", "omar@customer.test")
	byStatus := f.heldClaim(canceled, deals.ExtractedByDeterministic)

	if _, err := f.own.Add(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	f.WsExec(t, `UPDATE activity SET meeting_status = 'canceled' WHERE id = $1`, canceled)
	f.runPass()
	if a, b := f.met(byDomain), f.met(byStatus); a != "false" || b != "false" {
		t.Fatalf("after the domain was added and the meeting canceled the claims are met = %s, %s, want false, false", a, b)
	}

	if err := f.own.Remove(f.Admin(), "buyer-corp.test"); err != nil {
		t.Fatal(err)
	}
	f.WsExec(t, `UPDATE activity SET meeting_status = 'held' WHERE id = $1`, canceled)
	f.runPass()
	if a, b := f.met(byDomain), f.met(byStatus); a != "true" || b != "true" {
		t.Errorf("after the domain was removed and the meeting held again the claims are met = %s, %s, want true, true", a, b)
	}
}
