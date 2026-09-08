// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ensureLeadUpdateAuthority is the write gate in front of a lead update, and
// it asks a different question when the update hands the lead to somebody.
//
// An ordinary edit stays EnsureWritable's: a lead nobody owns is nobody's to
// rewrite. An ASSIGNMENT is the one act that must reach an ownerless lead —
// otherwise the queue every inbound lead lands in has no door out for a Team
// Lead, and the rep's only exit is the claim verb, which cannot hand a lead to
// a colleague.
//
// The exception is deliberately narrow. It admits an ownership-ONLY patch, so
// the ownerless arm never becomes a way to rewrite a lead's score, status or
// identity on the way past. A caller who may already write the row keeps every
// field, exactly as before; a caller who may not gets the owner and nothing
// else.
func ensureLeadUpdateAuthority(ctx context.Context, tx pgx.Tx, id ids.LeadID, in UpdateLeadInput) error {
	if in.OwnerID == nil {
		return auth.EnsureWritable(ctx, tx, "lead", id.UUID)
	}
	// The LOCK comes before the decision, the same order ClaimOwnership takes
	// and for the same reason. Whether this lead is ownerless is the fact the
	// whole gate turns on, and reading it unlocked leaves an interval in which
	// somebody else claims it: two reps both pass the ownerless arm, both
	// write, and the second silently takes a lead off the first — a record
	// neither rep could otherwise have touched. Holding the row makes the
	// loser's gate see the winner's name.
	if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly); err != nil {
		return err
	}
	if !ownershipOnlyLeadUpdate(in) {
		// Mixed content: the write arm answers, so an ownerless lead refuses
		// the whole patch rather than admitting the fields riding along with
		// the owner.
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
	}
	return auth.EnsureAssignable(ctx, tx, "lead", id.UUID, in.OwnerID.UUID)
}

// ownershipOnlyLeadUpdate answers whether this patch changes the owner and
// NOTHING else.
//
// Written as an exhaustive read of the input rather than a count of set
// fields: a new field added to UpdateLeadInput must be considered here, and a
// reader adding one sees the list it has to join. IfVersion and Trail are
// absent on purpose — a version precondition is not a field being written, and
// the trail names the write rather than changing the row.
func ownershipOnlyLeadUpdate(in UpdateLeadInput) bool {
	return len(in.Clear) == 0 && len(in.CustomFields) == 0 &&
		in.FullName == nil && in.Email == nil && in.Title == nil &&
		in.CompanyName == nil && in.CandidateOrgKey == nil &&
		in.Status == nil && in.Source == nil && in.Score == nil &&
		in.ScoreOverrideReason == nil && !in.ClearScoreOverride &&
		in.ProjectID == nil
}
