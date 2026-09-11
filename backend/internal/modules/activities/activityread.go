// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Reading the timeline: one activity, a page of them, the columns each read
// selects, and the links hung off the rows. The write side lives in
// activity.go — they were one file until it crossed the 500-line cap, and the
// seam between "record what happened" and "show what was recorded" is where
// the file wanted to come apart anyway.

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GetActivity reads one activity by id, opening its own transaction.
//
// Row scope and the held-row exclusion both live in readActivity, so every
// caller on this path gets them whether it opens the transaction or not.
func (s *Store) GetActivity(ctx context.Context, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.Activity{}, err
	}
	var out crmcontracts.Activity
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = readActivity(ctx, tx, id, archived)
		return err
	})
	return out, err
}

// orderClause is newest-first for every timeline read except the open-and-due
// task queue, which orders by the date the work is OWED rather than the date
// it was logged. A cap applied over the wrong order keeps the tasks most
// recently filed; capping THIS order keeps the tasks nearest their deadline,
// which is what a page capped at a dozen can actually afford to drop.
func orderClause(in ListActivitiesInput) string {
	if in.OpenAndDueBy != nil || in.OpenAndDueAfter != nil {
		return " ORDER BY a.due_at ASC, a.id ASC"
	}
	return " ORDER BY a.occurred_at DESC, a.id DESC"
}

// errOpenAndDueByWithCursor is refused rather than built into SQL. On THIS
// read, the keyset cursor is always decoded against the recency order
// (a.occurred_at, a.id) — storekit.Cursor's SortField/SortDesc exist for a
// query built through the sort-aware ListSort (listquery.go) to carry a
// different one, but this hand-built query does not mint or check them.
// Pairing a cursor with OpenAndDueBy, which runs under a different order
// (a.due_at ASC), would resume on an axis this query never validates and
// silently return the wrong rows rather than the next page. No caller does
// this today; the guard is here so the day one tries, it fails loudly instead
// of paginating wrongly. Routing this read through ListSort would let it mint
// a due_at-aware cursor and remove the need for this guard entirely — left
// for a follow-up rather than done here, since it touches the shared keyset
// path every other ListActivities caller also runs through.
var errOpenAndDueByWithCursor = errors.New(
	"activities: a cursor built for the recency order cannot resume an open-and-due read")

// ListActivities is the timeline read: newest first, optionally scoped to
// one entity through activity_link (the indexed 360-view join).
func (s *Store) ListActivities(ctx context.Context, in ListActivitiesInput) ([]crmcontracts.Activity, storekit.Page, error) {
	var activities []crmcontracts.Activity
	var page storekit.Page
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		// The colleague-domain snapshot for THIS read, taken beside the rows it
		// judges. ListActivitiesTx cannot take it itself: it is a free function
		// for a caller that already holds a transaction, so it has no seam to
		// ask — and a composite record read that carries none simply excludes
		// no sender, which is the same open default WithOwnDomains documents.
		if in.readerAddresses, err = s.readerAddressList(ctx, tx, readerOrNobody(ctx)); err != nil {
			return err
		}
		if in.ownDomains, err = s.ownDomainList(ctx, tx); err != nil {
			return err
		}
		// Measured only when the waiting filter is actually asked for. Every
		// activity list runs through here, and the spread is a percentile over
		// a year — a scan no read that never mentions waiting should pay for.
		if in.WaitingReplyAsOf != nil {
			if in.horizonDays, err = s.waitingHorizonFor(ctx, tx, *in.WaitingReplyAsOf); err != nil {
				return err
			}
		}
		activities, page, err = ListActivitiesTx(ctx, tx, in)
		return err
	})
	return activities, page, err
}

// ListActivitiesTx is ListActivities for a caller that already opened a
// transaction — the composite record read, whose timeline section must
// describe the same instant as its sibling sections. Same gate, same
// ordering; only the transaction is borrowed.
func ListActivitiesTx(ctx context.Context, tx pgx.Tx, in ListActivitiesInput) ([]crmcontracts.Activity, storekit.Page, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	// The record the timeline was narrowed TO is gated before it is filtered
	// on. The scope below is an ANY-LINK rule, so an activity linked to both a
	// visible person and a lead the caller may not read passes it — and
	// filtering on that lead's id would then answer "this lead exists, and here
	// is what happened on it" to someone with no right to either fact.
	if err := ensureNarrowingTargetVisible(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return nil, storekit.Page{}, err
	}
	if in.WithinProjectID != nil {
		if err := RequireProjectScope(ctx, tx, *in.WithinProjectID); err != nil {
			return nil, storekit.Page{}, err
		}
	}
	limit := storekit.ClampLimit(in.Limit)
	join, where, content, args, err := listActivitiesFilter(ctx, in)
	if err != nil {
		return nil, storekit.Page{}, err
	}

	rows, err := tx.Query(ctx,
		`SELECT `+activityColumns(content)+` FROM activity a`+join+` WHERE `+strings.Join(where, " AND ")+
			orderClause(in)+sprintf(" LIMIT %d", limit+1),
		args...)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	// Collected rather than streamed: attachLinks runs a second query on
	// this same transaction, which needs the cursor already closed.
	activities, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.Activity, error) {
		return scanActivity(row)
	})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var page storekit.Page
	if len(activities) > limit {
		activities = activities[:limit]
		if in.OpenAndDueBy == nil && in.OpenAndDueAfter == nil {
			last := activities[len(activities)-1]
			next, err := storekit.EncodeCursor(last.OccurredAt, ids.UUID(last.Id))
			if err != nil {
				return nil, storekit.Page{}, err
			}
			page = storekit.Page{HasMore: true, NextCursor: next}
		}
		// Else: the open-and-due order has no cursor to hand out — one is
		// never decoded against it (errOpenAndDueByWithCursor) — so it
		// reports no resumable page at all rather than a HasMore a caller has
		// no way to act on. Rows beyond the cap still exist; CountOpenForViewer
		// is the query that answers how many.
	}
	// What came with each message, on the same transaction and in one
	// statement — the batching rule attachLinks above already follows.
	if err := WithEmailRowFacts(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if err := attachLinks(ctx, tx, activities); err != nil {
		return nil, storekit.Page{}, err
	}
	if activities == nil {
		activities = []crmcontracts.Activity{}
	}
	return activities, page, nil
}

// readActivity is the module's ONE single-row activity read, and it
// carries the row scope itself. An activity has no owner_id and the
// workspace predicate bounds only the tenant, so its scope exists solely
// as the link-walk in
// auth.ActivityContentClause — a probe a call site can forget, and three
// lifecycle mutators did. Anything that returns a record is a read, so the
// gate lives here: an out-of-scope id reads as ErrNotFound, the same answer
// a missing row gives, whether the caller is getting, updating, archiving
// or relinking.
func readActivity(ctx context.Context, tx pgx.Tx, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	// DISCOVER-gated, like the list: a row the caller may know about is
	// read, and the audience decides per row whether its content comes
	// along (content_state). A caller who needs the content itself — a
	// send, an attachment, a transcript — asks readActivityContent.
	if err := auth.EnsureActivityVisible(ctx, tx, id.UUID); err != nil {
		return crmcontracts.Activity{}, err
	}
	return readActivityRow(ctx, tx, id, archived)
}

// readHeldActivity is readActivity for a caller in lockActivityForWrite's
// held branch, who already proved the row exists (the row lock) and that
// they hold write authority over it (auth.EnsureActivityWritableIn's
// ownership arm, asked independently of discoverability for exactly this
// reason). It skips readActivity's own DISCOVER gate deliberately:
// auth.ActivityAvailableClause folds `restricted_at IS NULL` into that gate
// unconditionally, so calling readActivity here would answer not-found no
// matter what the archived filter said — this read does not re-decide
// either question the caller already settled, only avoids asking a gate
// that would answer the wrong one. gates/restrictedreaders_test.go's
// call-graph walk credits it through lockActivityForWrite's own
// restricted_at check one hop away, rather than through a waiver entry.
func readHeldActivity(ctx context.Context, tx pgx.Tx, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	return readActivityRow(ctx, tx, id, archived)
}

// readActivityRow is the SELECT + scan + link attachment readActivity and
// readHeldActivity share; it carries no row-scope gate of its own, so every
// caller composes one first.
func readActivityRow(ctx context.Context, tx pgx.Tx, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	args := []any{id}
	arg := func(v any) int { args = append(args, v); return len(args) }
	content, err := auth.ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	q := `SELECT ` + activityColumns(content) + ` FROM activity a WHERE a.id = $1`
	if archived == storekit.LiveOnly {
		q += ` AND a.archived_at IS NULL`
	}
	a, err := scanActivity(tx.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Activity{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	one := []crmcontracts.Activity{a}
	if err := WithEmailRowFacts(ctx, tx, one); err != nil {
		return crmcontracts.Activity{}, err
	}
	if err := attachLinks(ctx, tx, one); err != nil {
		return crmcontracts.Activity{}, err
	}
	return one[0], nil
}

// attachLinks fills the contract's links[] on a page of activities in ONE
// query — the column the timeline's "via" chips and the per-person filter
// read. Batched rather than per-row because the timeline reads a page at a
// time.
//
// Each link row carries its OWN row-scope check, which the activity's does
// not subsume. Activity visibility is an ANY-link rule: one visible person
// makes the whole activity readable. Projecting every link row back would
// then disclose the ids of the other records it touches — a colleague's
// deal on the same thread — to a caller who cannot read them. A link whose
// target is out of scope is dropped, so links[] answers "what this is about,
// as far as you can see".
func attachLinks(ctx context.Context, tx pgx.Tx, activities []crmcontracts.Activity) error {
	if len(activities) == 0 {
		return nil
	}
	activityIDs := make([]ids.UUID, len(activities))
	byID := make(map[ids.UUID]int, len(activities))
	for i, a := range activities {
		activityIDs[i] = ids.UUID(a.Id)
		byID[ids.UUID(a.Id)] = i
	}
	args := []any{activityIDs}
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := auth.LinkTargetVisibleClause(ctx, "al", arg)
	if err != nil {
		return err
	}
	if visible == "" {
		visible = "TRUE"
	}
	rows, err := tx.Query(ctx, `
		SELECT al.id, al.activity_id, al.entity_type, `+linkIDCoalesceQualified("al")+`
		FROM activity_link al
		WHERE al.activity_id = ANY($1) AND `+visible+`
		ORDER BY al.activity_id, al.entity_type, al.id`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var linkID, activityID, entityID ids.UUID
		var entityType string
		if err := rows.Scan(&linkID, &activityID, &entityType, &entityID); err != nil {
			return err
		}
		i, ok := byID[activityID]
		if !ok {
			continue
		}
		link := crmcontracts.ActivityLink{
			Id:         (*openapi_types.UUID)(&linkID),
			ActivityId: (*openapi_types.UUID)(&activityID),
			EntityType: crmcontracts.ActivityLinkEntityType(entityType),
			EntityId:   openapi_types.UUID(entityID),
		}
		if activities[i].Links == nil {
			activities[i].Links = &[]crmcontracts.ActivityLink{}
		}
		*activities[i].Links = append(*activities[i].Links, link)
	}
	return rows.Err()
}

// scanActivity reads one row of the activity projection.
//
// Both the column list and these destinations come from activityProjection, so
// a column added or moved carries its own destination with it — the transposed
// scan that used to be possible among the string-ish neighbours has no second
// list to disagree with.
func scanActivity(row pgx.Row) (crmcontracts.Activity, error) {
	var s activityScan
	if err := row.Scan(activityScanTargets(&s)...); err != nil {
		return crmcontracts.Activity{}, err
	}
	return s.record(), nil
}

// CountActivities answers how many rows the SAME narrowing matches, with no
// bound on it.
//
// The count and the page share listActivitiesFilter, which is the point: a
// count assembled from its own copy of the WHERE clause would answer a question
// the list does not ask, and the two would drift apart one filter at a time.
// What differs is the projection and the absence of a LIMIT — a caller wanting
// both spends two statements, which is what a bounded page beside a true total
// costs.
//
// Gated exactly as the list is, in the same order: the object grant, then the
// narrowing target's own visibility, then the project scope. A count is a read
// of the rows it counts, and a number that moved when a colleague captured a
// contact the reader may not see would disclose that contact.
func (s *Store) CountActivities(ctx context.Context, in ListActivitiesInput) (int, error) {
	var total int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
			return err
		}
		if err := ensureNarrowingTargetVisible(ctx, tx, in.EntityType, in.EntityID); err != nil {
			return err
		}
		var err error
		if in.readerAddresses, err = s.readerAddressList(ctx, tx, readerOrNobody(ctx)); err != nil {
			return err
		}
		if in.ownDomains, err = s.ownDomainList(ctx, tx); err != nil {
			return err
		}
		if in.WithinProjectID != nil {
			if err := RequireProjectScope(ctx, tx, *in.WithinProjectID); err != nil {
				return err
			}
		}
		join, where, content, args, err := listActivitiesFilter(ctx, in)
		if err != nil {
			return err
		}
		// The list's own projection, counted. Not `count(*)` over the join
		// directly: the content clauses put placeholders in the SELECT list, and
		// a statement that binds an argument it never mentions is one Postgres
		// refuses outright ("could not determine data type of parameter"). The
		// planner reads the wrapper for what it is.
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM (SELECT `+activityColumns(content)+` FROM activity a`+join+
				` WHERE `+strings.Join(where, " AND ")+`) counted`, args...).Scan(&total)
	})
	return total, err
}
