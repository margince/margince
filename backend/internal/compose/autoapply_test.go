// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an automatic apply must REFUSE.
//
// The happy path is the easy half and the least informative: a gate that
// admitted everyone would pass it. These cases are the ones that say the pass
// declines where it cannot name an authority, and each is mutation-checked in
// the sense that reverting the branch it covers makes it fail.

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalWith is one pending proposal pointing where the case wants.
func approvalWith(targetType *string, targetID *ids.UUID) crmcontracts.Approval {
	a := crmcontracts.Approval{
		Id:               openapi_types.UUID(ids.NewV7()),
		Kind:             deals.CloseDateCorrectionKind,
		Status:           crmcontracts.ApprovalStatusPending,
		TargetEntityType: targetType,
	}
	if targetID != nil {
		id := openapi_types.UUID(*targetID)
		a.TargetEntityId = &id
	}
	return a
}

// The three kinds that apply themselves are the three the product decided on.
// A kind arriving here without that decision is the failure this pins: the list
// is the product's, not a refactor's.
func TestAutoApplyKindsAreTheThreeDecidedOn(t *testing.T) {
	want := map[string]bool{
		deals.CloseDateCorrectionKind: true,
		orgNameProposalKind:           true,
		lifecycleProposalKind:         true,
	}
	if len(autoApplyKinds) != len(want) {
		t.Fatalf("autoApplyKinds = %v, want exactly %d kinds", autoApplyKinds, len(want))
	}
	for _, kind := range autoApplyKinds {
		if !want[kind] {
			t.Errorf("%q applies itself but was never decided on — add it to the product decision, not just the list", kind)
		}
	}
}

// A sending kind must never reach the automatic path. The send cap in
// agentMayDecide would refuse it a second time, which is the point: this
// assertion holds the first refusal so the second never has to.
func TestAutoApplyKindsNeverSend(t *testing.T) {
	for _, kind := range autoApplyKinds {
		if approvals.ReleaseSends(kind) {
			t.Errorf("%q puts a message on the wire when approved, so it cannot apply itself", kind)
		}
	}
}

// Only target types whose owner this pass can resolve are eligible. A kind
// pointing at anything else has no authority to act under.
func TestAutoApplyResolvesOnlyOwnedTargets(t *testing.T) {
	for _, targetType := range []string{"deal", "organization"} {
		if !resolvableTarget(targetType) {
			t.Errorf("a %s target has an owner_id but is not resolvable", targetType)
		}
	}
	for _, targetType := range []string{"person", "activity", "lead", ""} {
		if resolvableTarget(targetType) {
			t.Errorf("a %q target is resolvable, but this pass names no owner column for it", targetType)
		}
	}
}

// applierFor builds an applier whose ports answer as the case wants, so a
// refusal can be driven without standing up six modules.
type applyPorts struct {
	undoable bool
	optedOut bool
	undoErr  error
	optErr   error
}

func (p applyPorts) applier() autoApplier {
	return autoApplier{
		undoable: func(context.Context, string, ids.UUID) (bool, error) {
			return p.undoable, p.undoErr
		},
		optedOut: func(context.Context, ids.UUID) (bool, error) {
			return p.optedOut, p.optErr
		},
	}
}

// A proposal naming no target cannot be applied: there is nothing to resolve an
// owner from, and applying it would be acting on a record the pass never read.
func TestAutoApplyRefusesAProposalWithNoTarget(t *testing.T) {
	a := applyPorts{undoable: true}.applier()
	applied, err := a.applyOne(context.Background(), approvalWith(nil, nil))
	if applied {
		t.Fatal("a proposal with no target was applied")
	}
	if err == nil {
		t.Fatal("a proposal with no target was accepted without a reason")
	}
}

// A target type no owner column is kept for is refused before any database
// work, by the ONE guard that knows which types can be read — recordOwner's.
//
// The applier carries a nil db on purpose: if the refusal did not come first,
// the statement would reach one and the case would panic rather than pass. That
// is the assertion, and it is why this drives applyOne rather than calling
// resolvableTarget directly, which would only restate the map to itself.
func TestAutoApplyRefusesAnUnownableTargetWithoutTouchingTheDatabase(t *testing.T) {
	a := applyPorts{undoable: true}.applier()
	id := ids.NewV7()
	applied, err := a.applyOne(context.Background(), approvalWith(ptrString("person"), &id))
	if applied {
		t.Fatal("a proposal against an unownable target was applied")
	}
	if err == nil {
		t.Fatal("an unownable target was accepted without a reason")
	}
	// The sentinel, not the message: the database error that follows a missing
	// guard also names the target type, so matching on text passes against the
	// very defect this exists to catch.
	if !errors.Is(err, errUnownableTarget) {
		t.Fatalf("refused for the wrong reason — the guard did not answer first: %v", err)
	}
}

// The sentinel separates "the person said no" from "the product could not",
// which are different facts about a queue that is not shrinking. A single
// counter for both would report an opt-out as a defect.
func TestOptedOutIsItsOwnAnswer(t *testing.T) {
	if errors.Is(errNoRecordOwner, errAutoApplyOptedOut) {
		t.Fatal("an unowned record reads as an opt-out; the two counts would merge")
	}
	if errors.Is(errOwnerHasNoAuthority, errAutoApplyOptedOut) {
		t.Fatal("an owner without authority reads as an opt-out; the two counts would merge")
	}
}
