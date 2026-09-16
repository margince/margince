// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"errors"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The ensure rule, branch by branch. It is what keeps the scan on demand:
// nothing is read twice, nothing is read for an account that did not move,
// and a busy account is read at most once an hour per reader.

var ensureNow = time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)

func settledRow(fingerprint string, ago time.Duration) *row {
	at := ensureNow.Add(-ago)
	return &row{Status: StatusDone, Fingerprint: &fingerprint, GeneratedAt: &at}
}

func TestAReaderWhoNeverAskedGetsARead(t *testing.T) {
	if got := decide(nil, "fp", ensureNow, false); got != queueRead {
		t.Errorf("decide = %v, want a queued read", got)
	}
}

// liveRow is a read whose current attempt began `ago` before ensureNow: a
// running one from its claim, a queued one from its request. A running read's
// request is ancient on purpose — it ages from its claim, and a fixture that
// dated both alike could not tell a read judged by the wrong instant.
func liveRow(status string, ago time.Duration) *row {
	at := ensureNow.Add(-ago)
	if status == StatusRunning {
		return &row{Status: status, RequestedAt: ensureNow.Add(-2 * time.Hour), StartedAt: &at}
	}
	return &row{Status: status, RequestedAt: at}
}

func TestAReadInFlightIsNeverStartedTwice(t *testing.T) {
	for _, status := range []string{StatusQueued, StatusRunning} {
		live := liveRow(status, ScanLease-time.Second)
		if got := decide(live, "fp", ensureNow, true); got != serveCurrent {
			t.Errorf("a %s read was started again (force or not): %v", status, got)
		}
	}
}

// A live read past its lease has no worker behind it: it was killed, timed
// out, or never claimed. Left alone the page would poll it forever and the
// rail would call it stalled forever, so opening the account is what starts
// it again — whether or not the reader forced.
func TestAReadNobodyIsWorkingAnyMoreIsReadAgain(t *testing.T) {
	for _, status := range []string{StatusQueued, StatusRunning} {
		for _, force := range []bool{false, true} {
			dead := liveRow(status, ScanLease+time.Second)
			if got := decide(dead, "fp", ensureNow, force); got != queueRead {
				t.Errorf("a %s read past its lease (force=%v) was served as in flight: %v", status, force, got)
			}
		}
	}
}

// A deferred read ages from the instant it resumes, not from the request an
// hour before it: a budget window parks a read for as long as the window is,
// and a park that counted as abandonment would re-queue it into the same
// window.
func TestADeferredReadIsNotAbandonedUntilItsResumeHasAged(t *testing.T) {
	resumes := ensureNow.Add(-time.Second)
	parked := &row{Status: StatusQueued, RequestedAt: ensureNow.Add(-2 * time.Hour), NextAttemptAt: &resumes}
	if parked.abandoned(ensureNow) {
		t.Error("a read parked two hours ago that resumed a second ago counts as abandoned")
	}
	if got := decide(parked, "fp", ensureNow, false); got != serveCurrent {
		t.Errorf("decide = %v, want the parked read served as in flight", got)
	}
	resumedLongAgo := ensureNow.Add(-ScanLease - time.Second)
	long := &row{Status: StatusQueued, RequestedAt: ensureNow.Add(-2 * time.Hour), NextAttemptAt: &resumedLongAgo}
	if !long.abandoned(ensureNow) {
		t.Error("a read whose resume passed a whole lease ago is still in flight")
	}
}

func TestAMatchingFingerprintIsServedWithoutAModelCall(t *testing.T) {
	if got := decide(settledRow("fp", 3*time.Hour), "fp", ensureNow, false); got != serveCurrent {
		t.Errorf("decide = %v, want the stored findings", got)
	}
}

func TestAChangedAccountUnderTheFloorIsServedStale(t *testing.T) {
	if got := decide(settledRow("old", 20*time.Minute), "new", ensureNow, false); got != serveStale {
		t.Errorf("decide = %v, want stale — a busy inbox must not re-read on every message", got)
	}
}

func TestAChangedAccountPastTheFloorIsReadAgain(t *testing.T) {
	if got := decide(settledRow("old", RescanFloor), "new", ensureNow, false); got != queueRead {
		t.Errorf("decide = %v, want a queued read once the floor has passed", got)
	}
}

func TestForceSkipsTheFloorAndTheFingerprintButNotTheInFlightCheck(t *testing.T) {
	if got := decide(settledRow("fp", time.Minute), "fp", ensureNow, true); got != queueRead {
		t.Errorf("force did not read a current account again: %v", got)
	}
}

func TestAFailedReadThatNeverSettledIsReadAgain(t *testing.T) {
	failed := &row{Status: StatusFailed}
	if got := decide(failed, "fp", ensureNow, false); got != queueRead {
		t.Errorf("decide = %v, want a queued read after a failure with nothing stored", got)
	}
}

// The merged list: the rules first, the model's after, one row per
// fingerprint, the cap reported.
func TestTheMergeKeepsOneRowPerSituationAndReportsTheCap(t *testing.T) {
	suggestion := func(fp string) crmcontracts.Company360Suggestion {
		return crmcontracts.Company360Suggestion{Fingerprint: fp}
	}
	rules := []crmcontracts.Company360Suggestion{suggestion("a"), suggestion("b")}
	read := []crmcontracts.Company360Suggestion{suggestion("b"), suggestion("c"), suggestion("d"), suggestion("e"), suggestion("f")}

	merged, dropped := applyCap(merge(rules, read))
	if len(merged) != maxAdvice || dropped != 1 {
		t.Fatalf("merged %d, dropped %d; want %d and 1", len(merged), dropped, maxAdvice)
	}
	if merged[0].Fingerprint != "a" || merged[1].Fingerprint != "b" || merged[2].Fingerprint != "c" {
		t.Errorf("order = %v; want the rules first, then the model's, each once", merged)
	}
}

// A scan is a reading aid for a contact. An agent holding a passport has the
// records themselves, and a call with no user has nobody to file the row
// under, so both are refused as permission rather than served as somebody's.
func TestTheScanBelongsToAHumanAndNobodyElse(t *testing.T) {
	svc := &Service{}
	agent := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:x"})
	if _, err := svc.caller(agent); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent: %v, want permission denied", err)
	}
	nobody := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:?"})
	if _, err := svc.caller(nobody); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a human with no user id: %v, want permission denied", err)
	}
	user := ids.NewV7()
	human := principal.WithActor(t.Context(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user})
	if got, err := svc.caller(human); err != nil || got.UUID != user {
		t.Errorf("a human: %v, %v; want their own id", got, err)
	}
}

// Retraction: a stored finding outlives the record it was written from. The
// scan runs, the reader archives the email, and the card must go with it —
// otherwise it quotes a subject the reader can no longer open and offers a
// "Create draft" the composer refuses.

func activityID(last byte) openapi_types.UUID {
	var raw [16]byte
	raw[15] = last
	return openapi_types.UUID(raw)
}

func citing(list ...openapi_types.UUID) crmcontracts.Company360Suggestion {
	return citingAs("fp", list...)
}

func citingAs(fingerprint string, list ...openapi_types.UUID) crmcontracts.Company360Suggestion {
	evidence := make([]crmcontracts.CompanyBriefEvidence, 0, len(list))
	for _, id := range list {
		evidence = append(evidence, crmcontracts.CompanyBriefEvidence{
			EntityType: crmcontracts.CompanyBriefEvidenceEntityTypeActivity,
			EntityId:   id,
		})
	}
	return crmcontracts.Company360Suggestion{Fingerprint: fingerprint, Evidence: evidence}
}

func standingSet(list ...openapi_types.UUID) map[ids.UUID]bool {
	out := map[ids.UUID]bool{}
	for _, id := range list {
		out[ids.UUID(id)] = true
	}
	return out
}

func TestAFindingWhoseOnlyCitedRecordIsGoneIsRetracted(t *testing.T) {
	gone := activityID(1)
	if kept := keepCited([]crmcontracts.Company360Suggestion{citing(gone)}, standingSet()); len(kept) != 0 {
		t.Errorf("kept %d findings; want the archived one retracted", len(kept))
	}
}

func TestAFindingWithOneStandingRecordAmongArchivedOnesStays(t *testing.T) {
	gone, live := activityID(1), activityID(2)
	kept := keepCited([]crmcontracts.Company360Suggestion{citing(gone, live)}, standingSet(live))
	if len(kept) != 1 {
		t.Errorf("kept %d findings; want the one with evidence left standing", len(kept))
	}
}

// A rule that fires on the account's shape rather than on one record has no
// citation to lose, so nothing about an archived email retracts it.
func TestAFindingCitingNoActivityIsNeverRetracted(t *testing.T) {
	shapeRule := crmcontracts.Company360Suggestion{Fingerprint: "no_next_step"}
	if kept := keepCited([]crmcontracts.Company360Suggestion{shapeRule}, standingSet()); len(kept) != 1 {
		t.Errorf("kept %d findings; want a rule with no citation untouched", len(kept))
	}
}

// The "Create draft" anchor is its own reason. The composer reads it live and
// refuses an archived one, so a finding still holding other evidence but
// anchored on a gone record offers a button that cannot work.
func TestAFindingWhoseDraftAnchorIsGoneIsRetracted(t *testing.T) {
	gone, live := activityID(1), activityID(2)
	finding := citing(live)
	finding.Action = &struct {
		//nolint:staticcheck // the generated contract's own field names.
		ActivityId *openapi_types.UUID `json:"activity_id,omitempty"`
		//nolint:staticcheck // as above.
		DealId *openapi_types.UUID                         `json:"deal_id,omitempty"`
		Kind   crmcontracts.Company360SuggestionActionKind `json:"kind"`
		Task   *crmcontracts.CreateTaskRequest             `json:"task,omitempty"`
	}{ActivityId: &gone, Kind: crmcontracts.Company360SuggestionActionKindDraftReply}

	if kept := keepCited([]crmcontracts.Company360Suggestion{finding}, standingSet(live)); len(kept) != 0 {
		t.Errorf("kept %d findings; want the one whose draft anchor is gone retracted", len(kept))
	}
}

// Nothing gone means nothing retracted — the ordinary read is untouched.
func TestNothingIsRetractedWhenEveryCitedRecordStands(t *testing.T) {
	first, second := activityID(1), activityID(2)
	findings := []crmcontracts.Company360Suggestion{citing(first), citing(second)}
	if kept := keepCited(findings, standingSet(first, second)); len(kept) != 2 {
		t.Errorf("kept %d findings; want both", len(kept))
	}
}

// Retraction runs before the cap, so a retracted finding does not hold a slot
// against a live one and the reported drop count stays the cap's own.
func TestARetractedFindingDoesNotHoldASlotAgainstALiveOne(t *testing.T) {
	gone := activityID(99)
	merged := make([]crmcontracts.Company360Suggestion, 0, maxAdvice+1)
	merged = append(merged, citing(gone))
	live := make([]openapi_types.UUID, 0, maxAdvice)
	for i := range maxAdvice {
		id := activityID(byte(i))
		live = append(live, id)
		merged = append(merged, citing(id))
	}

	findings, dropped := applyCap(keepCited(merged, standingSet(live...)))
	if len(findings) != maxAdvice || dropped != 0 {
		t.Fatalf("findings %d, dropped %d; want %d and 0", len(findings), dropped, maxAdvice)
	}
	for _, finding := range findings {
		if finding.Evidence[0].EntityId == gone {
			t.Errorf("the retracted finding survived the cap")
		}
	}
}

// Every activity a finding names, including the one only its ACTION names. The
// anchor is what the "Create draft" button opens, so a probe that asked only
// about evidence would leave the button unchecked — which is the control the
// defect was reported through.
func TestTheCitationCensusReachesTheActionAnchorAndDedupes(t *testing.T) {
	anchor, shared := activityID(1), activityID(2)
	withAnchor := citing(shared)
	withAnchor.Action = &struct {
		//nolint:staticcheck // the generated contract's own field names.
		ActivityId *openapi_types.UUID `json:"activity_id,omitempty"`
		//nolint:staticcheck // as above.
		DealId *openapi_types.UUID                         `json:"deal_id,omitempty"`
		Kind   crmcontracts.Company360SuggestionActionKind `json:"kind"`
		Task   *crmcontracts.CreateTaskRequest             `json:"task,omitempty"`
	}{ActivityId: &anchor, Kind: crmcontracts.Company360SuggestionActionKindDraftReply}

	cited := citedActivities([]crmcontracts.Company360Suggestion{withAnchor, citing(shared)})
	if len(cited) != 2 {
		t.Fatalf("cited = %v; want the anchor and the shared record, each once", cited)
	}
}
