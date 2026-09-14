// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// nameActivities turns a derived number's id array into named references.
//
// It lives in compose because the two producers cannot reach it themselves: a
// relationship strength and a lead score are both computed inside `contacts`,
// which may not import `activities`. Making them able to would put the activity
// gates inside a module that has no business holding them.
//
// ORDER is the caller's, not the database's. The ids arrive in the order they
// were counted — the order a reader is asked to recognise — and a result set
// re-ordered by whatever the planner chose would show one reader's receipts in
// a sequence matching nothing they were told.
//
// An id the reader cannot discover is OMITTED rather than rendered blank. The
// count stays the id array's own length at every call site, so the enriched
// list can be shorter than the number beside it: a number that shrank to what
// one reader may open would tell two readers different things about one score.
func nameActivities(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) ([]crmcontracts.ActivityReference, error) {
	// An empty slice, never nil. The caller stores a POINTER to what comes
	// back, and a nil slice serializes as `null` — which the contract's array
	// does not allow, and a strict client rejects. A score with no activities
	// is a real and common answer, so it arrives shaped like the empty list it
	// is.
	if len(activityIDs) == 0 {
		return []crmcontracts.ActivityReference{}, nil
	}
	// Deduped for the read and kept whole for the answer: one activity counted
	// twice is one row to fetch and, if the producer listed it twice, two
	// receipts to draw.
	wanted := make([]ids.UUID, 0, len(activityIDs))
	seen := make(map[ids.UUID]bool, len(activityIDs))
	for _, id := range activityIDs {
		if !seen[id] {
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	named, err := activities.ReferencesByID(ctx, tx, wanted)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.ActivityReference, 0, len(activityIDs))
	for _, id := range activityIDs {
		if ref, ok := named[id]; ok {
			out = append(out, ref)
		}
	}
	return out, nil
}

// namedActivitiesFor is nameActivities for a caller with no transaction of its
// own to lend — the strength handlers, whose read has already committed.
func namedActivitiesFor(
	ctx context.Context, pool *pgxpool.Pool, activityIDs []ids.UUID,
) ([]crmcontracts.ActivityReference, error) {
	var out []crmcontracts.ActivityReference
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = nameActivities(ctx, tx, activityIDs)
		return readErr
	})
	return out, err
}

// activityIDsOf is the wire's uuid array as the kernel's ids, which is the
// shape every gate below takes.
func activityIDsOf(wire []openapi_types.UUID) []ids.UUID {
	out := make([]ids.UUID, 0, len(wire))
	for _, id := range wire {
		out = append(out, ids.UUID(id))
	}
	return out
}
