// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"testing"
)

// A call confirmed in the conversation pins the version a staging of it would
// have pinned. The two are read by one function, so they agree, and a later
// edit moves the answer.
func TestPinTargetAnswersTheVersionAStagingWouldPin(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	in := e.mergeCall(e.seedTag(t, "PinSource"), e.seedTag(t, "PinSurvivor"))
	ctx := e.asPassport(passport)

	version, pinned, err := e.svc.PinTarget(ctx, in.Kind, in.TargetType, in.TargetID)
	if err != nil || !pinned {
		t.Fatalf("PinTarget = %d, pinned %v, err %v; want the tag's version pinned", version, pinned, err)
	}
	id, err := e.svc.Stage(ctx, in)
	if err != nil {
		t.Fatalf("staging the same call: %v", err)
	}
	var staged int64
	if err := e.owner.QueryRow(context.Background(), `SELECT target_version FROM approval WHERE id = $1`, id).Scan(&staged); err != nil {
		t.Fatalf("reading the staged pin: %v", err)
	}
	if staged != version {
		t.Errorf("PinTarget answered %d and the staging pinned %d, want the same version", version, staged)
	}

	if _, err := e.owner.Exec(context.Background(), `UPDATE tag SET version = version + 1 WHERE id = $1`, in.TargetID); err != nil {
		t.Fatalf("editing the tag: %v", err)
	}
	if moved, _, err := e.svc.PinTarget(ctx, in.Kind, in.TargetType, in.TargetID); err != nil || moved != version+1 {
		t.Errorf("after an edit PinTarget = %d (err %v), want %d", moved, err, version+1)
	}
}
