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
// nameActivityGroups below does the same for several arrays at one read.
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
	named, err := nameActivityGroups(ctx, tx, [][]ids.UUID{activityIDs})
	if err != nil {
		return nil, err
	}
	return named[0], nil
}

// nameActivityGroups names several id arrays against one read.
//
// A lead score decomposes into a factor per row and a row per entry, each
// naming its own activities. A group per factor would read the activity table
// once per factor, so the groups are read together and distributed after.
//
// Each group keeps the ordering and omission rules above, judged on its own
// ids. A group is answered as an empty slice rather than nil for the same
// reason a single call is.
func nameActivityGroups(
	ctx context.Context, tx pgx.Tx, groups [][]ids.UUID,
) ([][]crmcontracts.ActivityReference, error) {
	out := make([][]crmcontracts.ActivityReference, len(groups))
	for i := range out {
		out[i] = []crmcontracts.ActivityReference{}
	}
	// Deduped across every group as well as within one: an activity behind two
	// factors is one row to fetch and a receipt under each.
	wanted := make([]ids.UUID, 0)
	seen := make(map[ids.UUID]bool)
	for _, group := range groups {
		for _, id := range group {
			if !seen[id] {
				seen[id] = true
				wanted = append(wanted, id)
			}
		}
	}
	if len(wanted) == 0 {
		return out, nil
	}
	named, err := activities.ReferencesByID(ctx, tx, wanted)
	if err != nil {
		return nil, err
	}
	return distributeNamed(groups, named), nil
}

// distributeNamed gives each group its own ids, named.
//
// This is where the two rules above are applied. A group keeps the order its
// ids arrived in, and an id missing from the read is left out rather than
// rendered blank.
//
// It takes the read's result rather than doing the read, so both rules can be
// asserted without a database.
func distributeNamed(
	groups [][]ids.UUID, named map[ids.UUID]crmcontracts.ActivityReference,
) [][]crmcontracts.ActivityReference {
	out := make([][]crmcontracts.ActivityReference, len(groups))
	for i, group := range groups {
		refs := make([]crmcontracts.ActivityReference, 0, len(group))
		for _, id := range group {
			if ref, ok := named[id]; ok {
				refs = append(refs, ref)
			}
		}
		out[i] = refs
	}
	return out
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

// namedActivityGroupsFor is nameActivityGroups for a caller with no
// transaction of its own to lend, as namedActivitiesFor is for one group.
func namedActivityGroupsFor(
	ctx context.Context, pool *pgxpool.Pool, groups [][]ids.UUID,
) ([][]crmcontracts.ActivityReference, error) {
	// No transaction for a read with nothing to read. A score made only of fit
	// factors names no activities, and that is the common case rather than an
	// edge.
	if !anyActivityIn(groups) {
		return nameActivityGroups(ctx, nil, groups)
	}
	var out [][]crmcontracts.ActivityReference
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = nameActivityGroups(ctx, tx, groups)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// anyActivityIn reports whether any group names an activity.
func anyActivityIn(groups [][]ids.UUID) bool {
	for _, group := range groups {
		if len(group) > 0 {
			return true
		}
	}
	return false
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
