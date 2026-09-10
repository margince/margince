// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// When a lead's response clocks start and stop.
//
// Two stamps, both about the same promise: routed_at says when somebody became
// answerable for the lead, and first_response_at says when they answered. The
// SLA reads COALESCE(routed_at, created_at), so which of them is set — and
// when — is what decides whether a lead reads as breached.
//
// Split out of lead_update.go because it is one concept with its own rules, and
// because that file had grown past the length a reader can hold at once.

package people

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stampHumanFirstResponse adds the §18.1 first-response stamp to a patch
// that moves the lead off `new` by a HUMAN's hand. An agent's status change
// is not a response, and a lead already answered keeps its first stamp.
func stampHumanFirstResponse(ctx context.Context, p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	if in.Status == nil || current.Status != crmcontracts.LeadStatusNew || current.FirstResponseAt != nil {
		return nil
	}
	if LeadStatus(*in.Status) == LeadStatusNew {
		return nil
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	if actor.Type == principal.PrincipalHuman {
		p.Set(firstResponseColumn, nil, time.Now().UTC())
	}
	return nil
}

// leadStatusSetByColumn records who placed the lead on its current step.
const leadStatusSetByColumn = "status_set_by"

// startLeadResponseClockTx stamps routed_at on a lead that has just gained its
// first owner, unless something already started the clock.
//
// Two paths reach it — the update path's owner assignment and the claim
// button's — because the SLA reads COALESCE(routed_at, created_at) and a lead
// picked up long after it was captured would otherwise be measured from a date
// nobody was answerable for.
//
// Both guards live in the WHERE clause, which is what makes this safe to call
// from either path without a lock of its own: `routed_at IS NULL` means a
// concurrent claim and assignment cannot both stamp, and the loser writes
// nothing rather than moving a clock that had already started. `archived_at IS
// NULL` refuses a retired lead, whose deadline is nobody's business.
//
// The caller has already taken the row's write authority: the claim path
// through auth.EnsureClaimable inside storekit.ClaimOwnership, the update path
// through ensureLeadUpdateAuthority. This adds no probe of its own because it
// is the same transaction and the same row.
func startLeadResponseClockTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// The row is locked before it is read or written, so a concurrent claim and
	// assignment serialise here rather than racing on the WHERE clause. LiveOnly
	// refuses a retired lead outright, which is also the liveness answer this
	// write owes.
	if _, err := storekit.LockRow(ctx, tx, "lead", id, storekit.LiveOnly); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE lead SET routed_at = $2
		  WHERE id = $1 AND routed_at IS NULL AND archived_at IS NULL`,
		id, leadSLAClock().UTC()); err != nil {
		return fmt.Errorf("people: starting the lead response clock: %w", err)
	}
	return nil
}
