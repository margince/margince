// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A call confirmed in the conversation pins what a staging of it would have
// pinned. That is the target and the second row a tag merge folds into. A later
// edit to either moves the answer.
func TestPinRecordsAnswersThePinsAStagingWouldTake(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	in := e.mergeCall(e.seedTag(t, "PinSource"), e.seedTag(t, "PinSurvivor"))
	ctx := e.asPassport(passport)

	pins, err := e.svc.PinRecords(ctx, in)
	if err != nil || pins.Target == nil || pins.CoTarget == nil {
		t.Fatalf("PinRecords = %+v, err %v; want both tags pinned", pins, err)
	}
	if pins.Target.EntityID != in.TargetID || pins.CoTarget.EntityID != in.CoTargetID {
		t.Fatalf("PinRecords pinned %v and %v, want the source then the survivor", pins.Target.EntityID, pins.CoTarget.EntityID)
	}
	id, err := e.svc.Stage(ctx, in)
	if err != nil {
		t.Fatalf("staging the same call: %v", err)
	}
	var staged, stagedCo int64
	if err := e.owner.QueryRow(context.Background(),
		`SELECT target_version, co_target_version FROM approval WHERE id = $1`, id).Scan(&staged, &stagedCo); err != nil {
		t.Fatalf("reading the staged pins: %v", err)
	}
	if staged != pins.Target.Version || stagedCo != pins.CoTarget.Version {
		t.Errorf("PinRecords answered %d/%d and the staging pinned %d/%d, want the same",
			pins.Target.Version, pins.CoTarget.Version, staged, stagedCo)
	}

	if _, err := e.owner.Exec(context.Background(), `UPDATE tag SET version = version + 1 WHERE id = $1`, in.CoTargetID); err != nil {
		t.Fatalf("editing the survivor: %v", err)
	}
	moved, err := e.svc.PinRecords(ctx, in)
	if err != nil || moved.CoTarget == nil || moved.CoTarget.Version != pins.CoTarget.Version+1 {
		t.Errorf("after an edit to the survivor PinRecords = %+v (err %v), want its version moved", moved, err)
	}
}

// A kind staging declines to pin is not pinned by a confirmation either, and a
// call with no target has nothing to pin.
func TestPinRecordsKeepsTheStagingWaivers(t *testing.T) {
	e := setupStaging(t)
	ctx := e.asPassport(e.seedPassport(t))
	waived := e.mergeCall(e.seedTag(t, "WaivedSource"), e.seedTag(t, "WaivedSurvivor"))
	waived.Kind, waived.CoTargetType, waived.CoTargetID = approvalsubject.KindOwnershipTransfer, "", ids.UUID{}
	if pins, err := e.svc.PinRecords(ctx, waived); err != nil || pins.Target != nil || pins.CoTarget != nil {
		t.Errorf("an unpinned kind → %+v, err %v; want no pins", pins, err)
	}
	targetless := waived
	targetless.Kind, targetless.TargetType, targetless.TargetID = "merge_tags", "", ids.UUID{}
	if pins, err := e.svc.PinRecords(ctx, targetless); err != nil || pins.Target != nil {
		t.Errorf("a call with no target → %+v, err %v; want no pins", pins, err)
	}
}
