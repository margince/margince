// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// Which rows a staging binds itself to.
//
// A pin is what stops a confirmed act running over a record that changed after
// the human read the card. Taking one is a decision with two halves — WHICH
// rows the proposal's meaning rests on, and whether each can be re-checked at
// redemption — and both are answered here so a stager cannot answer either by
// omission.
//
// The re-check that consumes these lives in redeem.go, and the two have to be
// read together: a pin taken here that redemption cannot verify is a pin in
// name only.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// resolveTargetVersion reads the staged target's CURRENT version inside the
// staging transaction, so what a human approves is bound to the row as it
// stood when they were asked.
//
// The pin is taken here, at the ONE place every stager passes through, and
// never from what the caller supplied. A caller-supplied pin is a pin the
// caller can decline to supply: on the REST admission path it came from the
// optional If-Match header, so an agent that simply left the header off
// staged target_version NULL, and validateRedemptionTarget short-circuits on
// NULL — the approval then authorized the operation against whatever the row
// had drifted to inside the TTL, which for a body-less action route (send
// this offer) is any content state at all. Automation-staged actions carried
// no pin for the same reason: nothing had computed one.
//
// A target type outside versionTables has no version column to read, so it
// stays unpinned and the diff_hash identical-call binding is what holds. That
// residue is bounded and declared: TestConfirmFirstTargetsArePinnable holds
// the confirm-first surface to a ratified list of them.
// pinned is false for a target with no version column to read, and for a
// create, which has no prior row to bind to.
func resolveTargetVersion(ctx context.Context, tx pgx.Tx, in StageInput) (version int64, pinned bool, err error) {
	if in.TargetID.IsZero() || !TargetVersionCheckable(in.TargetType) {
		return 0, false, nil
	}
	// Two declared waivers, both meaning "this kind stages with no pin", and
	// each says a different thing about why: the target is context rather than
	// operand (contextTargetKinds), or it is the operand and the pin still binds
	// nothing the human judged (unpinnedKinds). Both are read here because this
	// is the one place a pin is taken.
	if TargetIsContextOnly(in.Kind) || TargetVersionUnpinned(in.Kind) {
		return 0, false, nil
	}
	current, err := targetVersion(ctx, tx, in.TargetType, in.TargetID)
	if err != nil {
		return 0, false, err
	}
	return current, true, nil
}

// resolveCoTargetVersion pins the SECOND row a proposal rests on.
//
// Same rule as the primary pin and deliberately not the same waivers: the
// kind-level exemptions above say something about the row a staging TARGETS —
// that it is context rather than operand, or that its version binds nothing a
// human judged. A co-target exists only where a caller declared that this
// proposal's meaning rests on that row too, so the declaration IS the claim
// that it should be pinned. What still applies is the table check: a type with
// no version column to read cannot be pinned by anyone, and minting a pin
// redemption could never verify is worse than not pinning.
func resolveCoTargetVersion(ctx context.Context, tx pgx.Tx, in StageInput) (version int64, pinned bool, err error) {
	if in.CoTargetID.IsZero() || in.CoTargetType == "" {
		return 0, false, nil
	}
	if !TargetVersionCheckable(in.CoTargetType) {
		return 0, false, fmt.Errorf(
			"approvals: %q cannot carry a version pin, so it cannot be a co-target: %w",
			in.CoTargetType, apperrors.ErrInvalidArgument)
	}
	current, err := targetVersion(ctx, tx, in.CoTargetType, in.CoTargetID)
	if err != nil {
		return 0, false, err
	}
	return current, true, nil
}
