// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// A decision waiting.
//
// The cases are about the lane never losing one: an unknown kind is shown
// anyway, a withheld queue is named rather than drawn empty, and a proposer
// spelled oddly is still named.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// whenStaged is a fixed instant: the lane carries the approval's own time, and a
// test reading the wall clock would prove nothing about that.
var whenStaged = time.Date(2026, time.March, 4, 7, 30, 0, 0, time.UTC)

type stubDecisions struct {
	approvals []crmcontracts.Approval
	err       error
}

func (s stubDecisions) PendingApprovals(
	context.Context, int,
) ([]crmcontracts.Approval, error) {
	return s.approvals, s.err
}

func stagedApproval(kind string) crmcontracts.Approval {
	return crmcontracts.Approval{
		Id:         openapi_types.UUID(uuid.New()),
		CreatedAt:  whenStaged,
		Kind:       kind,
		ProposedBy: "agent:overnight",
		Status:     crmcontracts.ApprovalStatus("pending"),
	}
}

func decisionsWaiting(
	t *testing.T, approvals ...crmcontracts.Approval,
) []crmcontracts.MagicLine {
	t.Helper()
	service := NewService(nil, nil, nil).WithPendingDecisions(stubDecisions{approvals: approvals})
	lines, refused, err := service.needsYou(context.Background(), 10)
	if err != nil {
		t.Fatalf("needsYou: %v", err)
	}
	if refused != nil {
		t.Fatalf("needsYou reported %q unavailable with nothing wrong", refused.Source)
	}
	return lines
}

// Each staged kind asks about something different, and the sentence is what the
// reader is asked. Two kinds sharing a key would ask one question twice.
func TestADecisionWaitingCarriesTheKindItAsksAbout(t *testing.T) {
	want := map[string]string{
		"coldstart":           "magic.action.approval_coldstart",
		"send_email":          "magic.action.approval_send_email",
		"advance_deal":        "magic.action.approval_advance_deal",
		"promote_lead":        "magic.action.approval_promote_lead",
		"overnight":           "magic.action.approval_overnight",
		"transcript_proposal": "magic.action.approval_transcript_proposal",
	}
	seen := make(map[string]string, len(want))
	for kind, key := range want {
		t.Run(kind, func(t *testing.T) {
			lines := decisionsWaiting(t, stagedApproval(kind))
			if len(lines) != 1 {
				t.Fatalf("lines = %d, want the one staged decision", len(lines))
			}
			if lines[0].Summary.Key != key {
				t.Errorf("summary key = %q, want %q", lines[0].Summary.Key, key)
			}
			if lines[0].Summary.Values == nil || (*lines[0].Summary.Values)["kind"] != kind {
				t.Errorf("summary values = %v, want the kind named", lines[0].Summary.Values)
			}
			if lines[0].Lane != crmcontracts.MagicLineLaneMagicLaneNeedsYou {
				t.Errorf("lane = %q, want needs_you", lines[0].Lane)
			}
			if !lines[0].OccurredAt.Equal(whenStaged) {
				t.Errorf("occurred at %v, want the moment it was staged", lines[0].OccurredAt)
			}
		})
		if first, clash := seen[key]; clash {
			t.Errorf("%q and %q ask the same question %q", first, kind, key)
		}
		seen[key] = kind
	}
}

// The inversion of this package's drop-the-unknown rule, and the reason for it:
// a done line dropped costs the reader a change they already have, a decision
// dropped costs them one they never learn is waiting.
func TestAnUnknownProposalKindIsStillShown(t *testing.T) {
	lines := decisionsWaiting(t, stagedApproval("a_kind_this_build_predates"))
	if len(lines) != 1 {
		t.Fatalf("lines = %d: an unrecognised kind was dropped, and with it a "+
			"decision the reader never learns is waiting", len(lines))
	}
	if lines[0].Summary.Key != "magic.action.approval_pending" {
		t.Errorf("summary key = %q, want the generic pending sentence", lines[0].Summary.Key)
	}
}

// "You may not see the queue" and "nothing is waiting" are opposite answers. A
// reader told the second on the day they lost the grant decides nothing and
// thinks they are done.
func TestAWithheldQueueIsNamedRatherThanEmpty(t *testing.T) {
	service := NewService(nil, nil, nil).WithPendingDecisions(
		stubDecisions{err: apperrors.ErrPermissionDenied},
	)
	lines, refused, err := service.needsYou(context.Background(), 10)
	if err != nil {
		t.Fatalf("a withheld queue failed the page: %v", err)
	}
	if refused == nil {
		t.Fatal("a withheld queue drew as an empty lane, which reads as nothing waiting")
	}
	if refused.Source != sourceStagedApproval {
		t.Errorf("source = %q, want %q", refused.Source, sourceStagedApproval)
	}
	if refused.Reason != crmcontracts.WorklistSourceUnavailableReasonWithheld {
		t.Errorf("reason = %q, want withheld", refused.Reason)
	}
	if len(lines) != 0 {
		t.Errorf("lines = %d, want none alongside a refusal", len(lines))
	}
}

// An installation that stages no approvals has nothing to decide, and a lane
// dark for want of wiring must not take the whole receipt down with it.
func TestAnUnwiredApprovalSeamDrawsAnEmptyLane(t *testing.T) {
	lines, refused, err := NewService(nil, nil, nil).needsYou(context.Background(), 10)
	if err != nil {
		t.Fatalf("an unbound seam refused the page: %v", err)
	}
	if refused != nil {
		t.Errorf("an unbound seam reported %q unavailable, which is a different "+
			"claim from having none", refused.Source)
	}
	if lines == nil || len(lines) != 0 {
		t.Errorf("lines = %v, want an empty slice the contract can serialise as a list", lines)
	}
}

// Each machine spelling is named as itself. A proposer filed under the wrong
// type is a claim about who asked, on the one surface whose purpose is to say.
func TestAMachineProposerIsNamedAsWhatItIs(t *testing.T) {
	cases := []struct {
		proposedBy string
		wantType   crmcontracts.MagicActorType
		wantID     string
	}{
		{"connector:gmail", crmcontracts.MagicActorTypeMagicActorConnector, "gmail"},
		{"agent:overnight", crmcontracts.MagicActorTypeMagicActorAgent, "overnight"},
		{"system:retention", crmcontracts.MagicActorTypeMagicActorSystem, "retention"},
		{"system", crmcontracts.MagicActorTypeMagicActorSystem, "system"},
	}
	for _, tc := range cases {
		t.Run(tc.proposedBy, func(t *testing.T) {
			approval := stagedApproval("overnight")
			approval.ProposedBy = tc.proposedBy
			lines := decisionsWaiting(t, approval)
			if len(lines) != 1 {
				t.Fatalf("lines = %d, want the one staged decision", len(lines))
			}
			if lines[0].Actor.Type != tc.wantType {
				t.Errorf("actor type = %q, want %q", lines[0].Actor.Type, tc.wantType)
			}
			if lines[0].Actor.Id != tc.wantID {
				t.Errorf("actor id = %q, want %q", lines[0].Actor.Id, tc.wantID)
			}
		})
	}
}

// THE PROVENANCE RULE, held. This surface reports what ran without being asked,
// and its actor vocabulary has no human member: a rep's own request for review
// reaches the inbox under their own name, and reporting it here as machinery
// would be a lie about who asked. An id this build cannot place is refused for
// the same reason — the likeliest thing it is, is a human.
func TestARequestAHumanMadeIsNotTheMachinerysWork(t *testing.T) {
	for _, proposedBy := range []string{"human:lars", "nonsense", "", "agent:", ":gmail"} {
		t.Run(proposedBy, func(t *testing.T) {
			approval := stagedApproval("overnight")
			approval.ProposedBy = proposedBy
			if lines := decisionsWaiting(t, approval); len(lines) != 0 {
				t.Errorf("a proposal by %q was drawn as %q machinery", proposedBy, lines[0].Actor.Type)
			}
		})
	}
}

// Expiry is lazy: the column still reads pending until the sweep runs, and the
// engine folds the deadline in on the way out. Drawing the raw column would
// count a dead proposal in the reader's total and send them to a decision the
// write refuses.
func TestAProposalThatRanOutOfTimeIsNotWaitingOnAnybody(t *testing.T) {
	for _, status := range []crmcontracts.ApprovalStatus{
		crmcontracts.ApprovalStatusExpired,
		crmcontracts.ApprovalStatusApproved,
		crmcontracts.ApprovalStatusRejected,
	} {
		t.Run(string(status), func(t *testing.T) {
			approval := stagedApproval("overnight")
			approval.Status = status
			if lines := decisionsWaiting(t, approval); len(lines) != 0 {
				t.Errorf("a %q proposal was drawn as a decision waiting", status)
			}
		})
	}
}

// Whose standing authority the proposal binds, where it binds one.
func TestAProposalStagedUnderARepsAuthoritySaysWhose(t *testing.T) {
	seat := openapi_types.UUID(uuid.New())
	approval := stagedApproval("advance_deal")
	approval.OnBehalfOf = &seat
	lines := decisionsWaiting(t, approval)
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want the one staged decision", len(lines))
	}
	if lines[0].Actor.OnBehalfOf == nil || *lines[0].Actor.OnBehalfOf != seat {
		t.Errorf("on behalf of = %v, want the seat whose authority it bound",
			lines[0].Actor.OnBehalfOf)
	}
}

// An entity reference carrying a type with no id points a reader at nothing, so
// a proposal with no target names no record at all.
func TestAProposalWithNoTargetNamesNoRecord(t *testing.T) {
	targetType := "deal"
	cases := map[string]crmcontracts.Approval{
		"neither half": stagedApproval("promote_lead"),
		"a type with no id": func() crmcontracts.Approval {
			a := stagedApproval("promote_lead")
			a.TargetEntityType = &targetType
			return a
		}(),
		"an id with no type": func() crmcontracts.Approval {
			a := stagedApproval("promote_lead")
			id := openapi_types.UUID(uuid.New())
			a.TargetEntityId = &id
			return a
		}(),
	}
	for name, approval := range cases {
		t.Run(name, func(t *testing.T) {
			lines := decisionsWaiting(t, approval)
			if len(lines) != 1 {
				t.Fatalf("lines = %d, want the one staged decision", len(lines))
			}
			if lines[0].Entity != nil {
				t.Errorf("entity = %+v, want none: half a reference points nowhere",
					*lines[0].Entity)
			}
		})
	}
}

// Both halves present, and the frozen caption travels with them.
func TestATargetedProposalNamesTheRecordAndItsCaption(t *testing.T) {
	targetType, label := "deal", "Northwind renewal"
	id := openapi_types.UUID(uuid.New())
	approval := stagedApproval("advance_deal")
	approval.TargetEntityType = &targetType
	approval.TargetEntityId = &id
	approval.TargetLabel = &label
	lines := decisionsWaiting(t, approval)
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want the one staged decision", len(lines))
	}
	if lines[0].Entity == nil {
		t.Fatal("entity = none, want the record the effect would mutate")
	}
	if lines[0].Entity.Type != targetType || lines[0].Entity.Id != id {
		t.Errorf("entity = %+v, want %s %s", *lines[0].Entity, targetType, id)
	}
	if lines[0].Summary.Values == nil || (*lines[0].Summary.Values)["target"] != label {
		t.Errorf("summary values = %v, want the staged caption", lines[0].Summary.Values)
	}
}

// Nothing has happened yet, so there is nothing to put back — said out loud
// rather than left absent for a client to guess about.
func TestADecisionNotYetMadeHasNothingToTakeBack(t *testing.T) {
	lines := decisionsWaiting(t, stagedApproval("coldstart"))
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want the one staged decision", len(lines))
	}
	undo := lines[0].Undo
	if undo == nil {
		t.Fatal("undo = absent, want a stated policy a client can draw")
	}
	if undo.Undoable {
		t.Error("a decision nobody has made is offered as undoable")
	}
	if undo.Reason == nil || *undo.Reason != nothingToUndo {
		t.Errorf("undo reason = %v, want %q", undo.Reason, nothingToUndo)
	}
}

// Anything that is not a refusal is a real failure, and a lane that swallowed it
// would report a healthy morning over a broken read.
func TestAFailedApprovalReadReachesTheCaller(t *testing.T) {
	boom := errors.New("the approvals store is unreachable")
	service := NewService(nil, nil, nil).WithPendingDecisions(stubDecisions{err: boom})
	lines, refused, err := service.needsYou(context.Background(), 10)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the read's own failure", err)
	}
	if lines != nil || refused != nil {
		t.Errorf("lines = %v and refusal = %v alongside a failure, want neither", lines, refused)
	}
}
