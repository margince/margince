// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A scheduled message the engine refuses when it comes due holds and opens the
// review the refusal owes, so the rep has something to confirm. Only an engine
// refusal opens one; a missed window or a technical fault holds alone.

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reviewsFor counts every review bound to one message, and the live ones.
func (p *preflightEnv) reviewsFor(t *testing.T, id ids.UUID) (all, live int) {
	t.Helper()
	if err := p.Owner.QueryRow(context.Background(), `
		SELECT count(*), count(*) FILTER (WHERE resolved_at IS NULL)
		  FROM communication_review WHERE delivery_intent_id = $1`, id).Scan(&all, &live); err != nil {
		t.Fatalf("counting the reviews for %s: %v", id, err)
	}
	return all, live
}

// liveReviewFor reads the one live review bound to a message.
func (p *preflightEnv) liveReviewFor(t *testing.T, id ids.UUID) string {
	t.Helper()
	var review string
	if err := p.Owner.QueryRow(context.Background(), `
		SELECT id::text FROM communication_review
		 WHERE delivery_intent_id = $1 AND resolved_at IS NULL`, id).Scan(&review); err != nil {
		t.Fatalf("reading the live review for %s: %v", id, err)
	}
	return review
}

// heldByAnObjectionAtFire schedules a clean message, records the recipient's
// withdrawal after it was scheduled, and fires it.
func heldByAnObjectionAtFire(t *testing.T) (*preflightEnv, ids.UUID) {
	t.Helper()
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	id := p.scheduleFor(t, time.Now().Add(2*time.Hour))
	p.withdrawConsent(t)
	p.makeDue(t, id)
	p.fire(t, id)
	return p, id
}

// confirmReview directs the held message the way the review modal does. It
// reads the review first and sends back the revision and reasons it showed.
func (p *preflightEnv) confirmReview(t *testing.T, review string) int {
	t.Helper()
	var served struct {
		AssessmentRevision string `json:"assessment_revision"`
		ReasonCode         string `json:"reason_code"`
	}
	if status := p.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &served); status != http.StatusOK {
		t.Fatalf("opening the review → %d", status)
	}
	return p.Call(t, "POST", "/v1/communication-reviews/"+review+"/direct-send", AnyMap{
		"reason_code":         "other",
		"explanation":         "The buyer asked for this confirmation on the phone this morning.",
		"warning_version":     consent.OverrideWarningVersion,
		"acknowledged":        true,
		"shown_reasons":       []string{served.ReasonCode},
		"assessment_revision": served.AssessmentRevision,
	}, nil, nil)
}

// The main path. Held with one review and nothing sent; the review is offered
// on the held row; confirming it sends the message once.
//
// Mutation: drop the recordReviewForHoldTx call from the fire's deferred hold
// (scheduledsendfire.go) and this fails on the review count.
func TestARefusalAtFireOpensAReviewTheRepCanConfirm(t *testing.T) {
	p, id := heldByAnObjectionAtFire(t)

	status, reason := p.scheduledStatus(t, id)
	if status != activities.ScheduledStatusHeld || reason != activities.HeldConsentWithdrawn {
		t.Fatalf("the refused message reads %q/%q, want held/%s", status, reason, activities.HeldConsentWithdrawn)
	}
	if all, live := p.reviewsFor(t, id); all != 1 || live != 1 {
		t.Fatalf("%d review(s), %d live, for a message refused at fire; want exactly one live review", all, live)
	}
	if got := p.countDeliveries(t); got != 0 {
		t.Fatalf("a held message staged %d deliveries, want 0", got)
	}
	review := p.liveReviewFor(t, id)

	var row struct {
		ReviewID      *string  `json:"review_id"`
		ReviewActions []string `json:"review_actions"`
	}
	if code := p.Call(t, "GET", "/v1/scheduled-sends/"+id.String(), nil, nil, &row); code != http.StatusOK {
		t.Fatalf("reading the held message → %d", code)
	}
	if row.ReviewID == nil || *row.ReviewID != review {
		t.Fatalf("the held row names review %v, want %s", row.ReviewID, review)
	}
	if len(row.ReviewActions) != 1 || row.ReviewActions[0] != "direct_send" {
		t.Fatalf("the held row offers %v, want [direct_send] for a rep who may direct it", row.ReviewActions)
	}

	card, found := p.heldCardFor(t, id)
	if !found {
		t.Fatal("the hold raised no card in the rep's inbox")
	}
	if code := p.confirmReview(t, review); code != http.StatusCreated {
		t.Fatalf("confirming the review → %d, want 201", code)
	}
	if got := p.countDeliveries(t); got != 1 {
		t.Fatalf("%d deliveries after one confirmation, want exactly 1", got)
	}
	if status, _ := p.scheduledStatus(t, id); status != activities.ScheduledStatusReleased {
		t.Fatalf("the confirmed message reads %q, want released", status)
	}
	if _, live := p.reviewsFor(t, id); live != 0 {
		t.Errorf("%d review(s) still live after the message went", live)
	}
	if _, still := p.heldCardFor(t, id); still {
		t.Error("the held card still asks about a message that has gone")
	}
	var withdrawnFor string
	if err := p.Owner.QueryRow(context.Background(), `
		SELECT evidence->>'reason' FROM audit_log
		 WHERE entity_type = 'approval' AND entity_id = $1 AND after->>'status' = 'expired'`,
		card.ID).Scan(&withdrawnFor); err != nil {
		t.Fatalf("reading why the card was withdrawn: %v", err)
	}
	if withdrawnFor != "the message was sent after its review was confirmed" {
		t.Errorf("the card was withdrawn as %q, which is not what happened to the message", withdrawnFor)
	}
	// A second press is refused and sends nothing more.
	if code := p.confirmReview(t, review); code == http.StatusCreated {
		t.Error("confirming a resolved review sent the message again")
	}
	if got := p.countDeliveries(t); got != 1 {
		t.Fatalf("%d deliveries after a repeated confirmation, want still 1", got)
	}
}

// A retried alarm, and a message re-armed and refused again, keep one review.
func TestARefusalFiredAgainKeepsOneReview(t *testing.T) {
	p, id := heldByAnObjectionAtFire(t)
	p.fire(t, id)

	// The rep re-arms it without fixing anything; the next fire refuses again.
	p.rescheduleTo(t, id, time.Now().Add(time.Hour))
	p.makeDue(t, id)
	p.fire(t, id)
	p.fire(t, id)

	if status, _ := p.scheduledStatus(t, id); status != activities.ScheduledStatusHeld {
		t.Fatalf("the re-refused message reads %q, want held", status)
	}
	if all, live := p.reviewsFor(t, id); all != 1 || live != 1 {
		t.Fatalf("%d review(s), %d live, after three fires; want one review throughout", all, live)
	}
}

// A cancellation racing the fire ends cancelled with no live review and no
// delivery, whichever of the two commits first.
func TestACancelRacingARefusedFireLeavesNoLiveReview(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	id := p.scheduleFor(t, time.Now().Add(2*time.Hour))
	p.withdrawConsent(t)
	p.makeDue(t, id)
	ws := p.workspaceID(t)

	var wg sync.WaitGroup
	var fireErr error
	var cancelStatus int
	wg.Add(2)
	go func() {
		defer wg.Done()
		fireErr = compose.DriveScheduledSendForTest(context.Background(), p.Pool, ws, id,
			compose.SendOrigin{PublicBaseURL: preflightBaseURL})
	}()
	go func() {
		defer wg.Done()
		cancelStatus = p.Call(t, "POST", "/v1/scheduled-sends/"+id.String()+"/cancel", nil, nil, nil)
	}()
	wg.Wait()
	if fireErr != nil {
		t.Fatalf("the fire failed: %v", fireErr)
	}
	if cancelStatus != http.StatusNoContent {
		t.Fatalf("cancelling → %d, want 204", cancelStatus)
	}
	if status, _ := p.scheduledStatus(t, id); status != activities.ScheduledStatusCancelled {
		t.Fatalf("the message reads %q, want cancelled", status)
	}
	if _, live := p.reviewsFor(t, id); live != 0 {
		t.Errorf("%d live review(s) for a cancelled message", live)
	}
	if got := p.countDeliveries(t); got != 0 {
		t.Errorf("%d deliveries for a cancelled message", got)
	}
}

// A message rescheduled before its alarm rings is not held or reviewed under
// the old moment; the alarm snoozes to the new one.
func TestARescheduleBeforeTheFireOpensNoReview(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
	id := p.scheduleFor(t, time.Now().Add(2*time.Hour))
	p.withdrawConsent(t)
	p.rescheduleTo(t, id, time.Now().Add(48*time.Hour))
	p.fire(t, id)

	if status, _ := p.scheduledStatus(t, id); status != activities.ScheduledStatusScheduled {
		t.Fatalf("the moved message reads %q, want still scheduled", status)
	}
	if all, _ := p.reviewsFor(t, id); all != 0 {
		t.Errorf("%d review(s) for a message whose new moment has not come", all)
	}
}

// A technical fault and a missed window hold with their own reasons and open
// no review: nothing about them is the engine's to decide.
func TestHoldsThatAreNotRefusalsOpenNoReview(t *testing.T) {
	t.Run("technical fault", func(t *testing.T) {
		p := setupPreflight(t)
		p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
		id := p.scheduleFor(t, time.Now().Add(2*time.Hour))
		p.makeDue(t, id)
		// A payload this build cannot read is a fault, not a refusal.
		if _, err := p.Owner.Exec(context.Background(),
			`UPDATE scheduled_send SET payload_version = payload_version + 100 WHERE id = $1`, id); err != nil {
			t.Fatal(err)
		}
		ws := p.workspaceID(t)
		origin := compose.SendOrigin{PublicBaseURL: preflightBaseURL}
		if err := compose.DriveScheduledSendForTest(context.Background(), p.Pool, ws, id, origin); err == nil {
			t.Fatal("a fire over an unreadable payload reported success")
		}
		// The last rung of the ladder holds it instead of retrying.
		if err := compose.DriveLastScheduledSendAttemptForTest(context.Background(), p.Pool, ws, id, origin); err != nil {
			t.Fatalf("the last attempt did not hold the faulted message: %v", err)
		}
		if status, reason := p.scheduledStatus(t, id); status != activities.ScheduledStatusHeld ||
			reason != activities.HeldTimerExhausted {
			t.Fatalf("the faulted message reads %q/%q, want held/%s", status, reason, activities.HeldTimerExhausted)
		}
		if all, _ := p.reviewsFor(t, id); all != 0 {
			t.Errorf("%d review(s) for a technical fault", all)
		}
	})
	t.Run("missed window", func(t *testing.T) {
		p := setupPreflight(t)
		p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
		id := p.scheduleFor(t, time.Now().Add(2*time.Hour))
		p.withdrawConsent(t)
		p.setDueAt(t, id, time.Now().Add(-4*time.Hour))
		p.fire(t, id)
		if _, reason := p.scheduledStatus(t, id); reason != activities.HeldMissedWindow {
			t.Fatalf("held for %q, want %s", reason, activities.HeldMissedWindow)
		}
		if all, _ := p.reviewsFor(t, id); all != 0 {
			t.Errorf("%d review(s) for a missed window", all)
		}
	})
}

// A row held at fire takes a spelling correction under the same approval, as
// a row held at the keyboard does. An interrupted send leaves the approval open.
func TestARowHeldAtFireTakesAWordingCorrection(t *testing.T) {
	p, id := heldByAnObjectionAtFire(t)
	review := p.liveReviewFor(t, id)
	if _, err := p.Owner.Exec(t.Context(), `CREATE FUNCTION reject_fire_wording_staging() RETURNS trigger LANGUAGE plpgsql
		AS $$ BEGIN RAISE EXCEPTION 'delivery staging interrupted by the test'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Owner.Exec(t.Context(), `CREATE TRIGGER reject_fire_wording_staging BEFORE UPDATE ON communication_instruction
		FOR EACH ROW WHEN (NEW.status='consumed') EXECUTE FUNCTION reject_fire_wording_staging()`); err != nil {
		t.Fatal(err)
	}
	if code := p.confirmReview(t, review); code != http.StatusInternalServerError {
		t.Fatalf("the interrupted confirmation → %d, want 500", code)
	}
	if _, err := p.Owner.Exec(t.Context(), `DROP FUNCTION reject_fire_wording_staging() CASCADE`); err != nil {
		t.Fatal(err)
	}
	var current crmcontracts.CommunicationReview
	if code := p.Call(t, "GET", "/v1/communication-reviews/"+review, nil, nil, &current); code != http.StatusOK {
		t.Fatalf("reading the review → %d", code)
	}
	if current.WordingCorrection == nil {
		t.Fatal("the review offers no wording correction on a row held at fire")
	}
	corrected := spellingCorrection(*current.WordingCorrection)
	corrected["body"] = "Written the night before!"
	delete(corrected, "html_body")
	var result crmcontracts.WordingCorrectionResult
	if code := p.Call(t, "POST", "/v1/communication-reviews/"+review+"/correct-wording",
		corrected, nil, &result); code != http.StatusOK {
		t.Fatalf("correcting the wording of a row held at fire → %d, want 200", code)
	}
	if result.Version != current.WordingCorrection.Version+1 {
		t.Errorf("the correction answered version %d, want %d", result.Version, current.WordingCorrection.Version+1)
	}
}
