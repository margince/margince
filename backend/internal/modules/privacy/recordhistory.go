// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Actor-type and action literals that recur across this file (map key,
// switch case, occurrence count 3+ once this file joins fieldhistory.go/
// retention.go) — named once so goconst has one extraction target instead
// of flagging each new occurrence.
const (
	actorTypeAgent     = "agent"
	actorTypeHuman     = "human"
	actorTypeSystem    = "system"
	actorTypeConnector = "connector"
	actionArchive      = "archive"
	actionCreate       = "create"
	actionUpdate       = "update"
)

// RecordHistoryFilter carries the validated query surface of
// (GET /records/{entity_type}/{id}/history).
type RecordHistoryFilter struct {
	EntityType string
	EntityID   ids.UUID
	Cursor     *string
	Limit      *int
	// Action narrows to one audit verb. A caller after ONE event asks for it
	// rather than paging the trail looking for it — a walk whose length is the
	// record's whole history, and whose first page is a confidently wrong
	// answer on exactly the records somebody worked hardest.
	Action *string
}

// RecordHistoryEntry is one audit_log row rendered as a history line —
// the whole-mutation view, one entry per row (field-history's per-field
// projection is the sibling read). Before/After are the row's own field
// images with the entity's mask applied by omission, and are absent on a row
// about a LINK: those images hold the link's columns and not the record's, and
// Edge is what a reader is given there instead. Scrub tombstones and
// retention rows carry their operational tallies on audit_log.evidence, of
// which this read projects the reversal link and never the blob; other meta
// verbs' before/after payloads are served verbatim—workspace-operational
// context behind the same record-read gate, never subject PII.
type RecordHistoryEntry struct {
	ID                ids.UUID
	ActorType         string
	ActorID           string
	ActorName         *string
	OnBehalfOf        *ids.UUID
	OnBehalfOfName    *string
	Action            string
	OccurredAt        time.Time
	AuthorizationRule *string
	Before            map[string]any
	After             map[string]any
	Summary           string
	// AgentClient is the tool's registered name when the write came through an
	// OAuth grant. Nil for a hand-minted passport and for anything undelegated;
	// the Summary line already reads correctly either way, and this is here so
	// a client rendering its own sentence does not have to parse one.
	AgentClient *string
	// UndidAuditLogID is the entry this one REVERSES, so a reader can pair the
	// two instead of counting a reversal as a fresh change. Nil on every row a
	// restore did not write. The opposite direction is not here: a row that HAS
	// been reversed says so through its undoability answer, computed for the
	// whole record rather than for one page.
	UndidAuditLogID *ids.UUID
	// Edge is the LINK this entry changed, with the other end named. Nil on
	// every row that changed a field of the record itself, which is what a
	// reader needs to tell the two apart: an edge line has no field diff to
	// draw, because the fields it moved are the link's and not this record's.
	Edge *HistoryEdge
}

// RecordHistoryPage is one keyset window of the timeline, NEWEST first — the
// same order as field-history's diff feed, and the order the surface needs.
//
// A record's history is read to answer "what just happened", and the change a
// person wants to put back is almost always the last one. Oldest-first buried
// it at the bottom of the page and put the reader's own edit furthest from
// them; it also meant page one held the oldest twenty rows, so no amount of
// reversing at the surface could fix the order.
type RecordHistoryPage struct {
	Entries    []RecordHistoryEntry
	NextCursor string
	HasMore    bool
}

// recordAuditRow carries one scanned audit_log row plus its resolved
// display names, ready for the pure entry transform.
type recordAuditRow struct {
	id                ids.UUID
	actorType         string
	actorID           string
	onBehalfOf        *ids.UUID
	action            string
	occurredAt        time.Time
	authorizationRule *string
	before            map[string]any
	after             map[string]any
	passportID        *ids.UUID
	actorDisplayName  *string
	onBehalfOfName    *string
	// agentClientName is the tool's own name, when the passport behind the row
	// came from an OAuth grant. Nil for a hand-minted passport, for a deleted
	// client, and for every write that was not delegated.
	agentClientName *string
	// undidAuditLogID names the row this one reversed; nil on every row that was
	// not written by a restore.
	undidAuditLogID *ids.UUID
	// edge is the LINK this row changed, with the other end named — nil on every
	// row that changed a field of the record itself.
	edge *edgeSubject
}

// recordHistoryEntry renders one audit row as a history entry: mask both
// payload sides by omission (hiding history and live value is one motion),
// then compose the plain-language line. The actor display falls back to
// the raw prefixed actor_id when no app_user resolves — an honest
// identifier, never an invented name; agent/connector/system ids never
// resolve, and their lines render from their own branch (an agent's human
// context is the on-behalf-of authority, nothing else).
func recordHistoryEntry(row recordAuditRow, mask entityFieldMask) RecordHistoryEntry {
	display := row.actorID
	if row.actorDisplayName != nil && *row.actorDisplayName != "" {
		display = *row.actorDisplayName
	}
	before, after := applyFieldMask(row.before, mask), applyFieldMask(row.after, mask)
	summary := composeRecordSummary(row.actorType, display, row.onBehalfOfName,
		row.action, row.passportID != nil, row.agentClientName)
	var edge *HistoryEdge
	if row.edge != nil {
		subject := recordSummarySubject(row.actorType, display, row.onBehalfOfName,
			row.passportID != nil, row.agentClientName)
		if line, phrased := edgeSummary(subject, row.action, *row.edge, after); phrased {
			summary = line
		}
		rendered := historyEdgeOf(*row.edge)
		edge = &rendered
		// The images belong to the LINK — role, the dates, the primary flag — and
		// the line above is where they are named as such. Carried onto the entry
		// they are served as this RECORD's before/after, where a consumer reads
		// them as changes to fields the record has never had.
		before, after = nil, nil
	}
	return RecordHistoryEntry{
		Edge:              edge,
		AgentClient:       row.agentClientName,
		ID:                row.id,
		ActorType:         row.actorType,
		ActorID:           row.actorID,
		ActorName:         row.actorDisplayName,
		OnBehalfOf:        row.onBehalfOf,
		OnBehalfOfName:    row.onBehalfOfName,
		Action:            row.action,
		OccurredAt:        row.occurredAt,
		AuthorizationRule: row.authorizationRule,
		UndidAuditLogID:   row.undidAuditLogID,
		Before:            before,
		After:             after,
		Summary:           summary,
	}
}

// admitRecordHistoryRead is the object-level half of this read's gate stack:
// a human session, a known entity kind, and the read grant on it. The ROW-level
// half runs inside the transaction, because visibility is a statement.
//
// Separated from ListRecordHistory so the admission rules read as one list
// rather than as a preamble to paging. Both jobs are short; together they were
// one function doing two.
func admitRecordHistoryRead(ctx context.Context, f RecordHistoryFilter) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return apperrors.ErrPermissionDenied
	}
	if !fieldHistoryEntityTypes[f.EntityType] {
		return fmt.Errorf("record-history entity %q: %w", f.EntityType, apperrors.ErrNotFound)
	}
	return auth.Require(ctx, f.EntityType, principal.ActionRead)
}

// ListRecordHistory reads one record's whole-mutation timeline — every
// audit verb, one line per row — inside a single workspace tx. The gate
// stack is ListFieldHistory's, verbatim: a human session, object-level
// read on the entity type, and the row-scope visibility check (out of
// scope reads as not-found, indistinguishable from not-there).
//
// Unlike field-history there is no action allowlist: a merge or export
// line IS the point of this view. Payload honesty is inherited instead —
// scrub tombstones and retention rows carry operational tallies on
// audit_log.evidence, which this read never selects; other meta verbs
// (merge, promote, export, record_share, enrich, coldstart) serve
// operational context verbatim from before/after, workspace-operational
// data behind the same record-read gate, never subject PII.
func ListRecordHistory(ctx context.Context, db *database.DB, f RecordHistoryFilter) (RecordHistoryPage, error) {
	if err := admitRecordHistoryRead(ctx, f); err != nil {
		return RecordHistoryPage{}, err
	}

	limit := storekit.ClampLimit(f.Limit)
	var cursor storekit.Cursor
	useCursor := false
	if f.Cursor != nil && *f.Cursor != "" {
		c, err := storekit.DecodeCursor(*f.Cursor)
		if err != nil {
			return RecordHistoryPage{}, err
		}
		cursor, useCursor = c, true
	}
	mask := defaultFieldMasks[f.EntityType]

	page := RecordHistoryPage{Entries: []RecordHistoryEntry{}}
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		// activity carries no owner_id — it row-scopes through its links,
		// so its visibility check dispatches to EnsureActivityContentVisible.
		var visErr error
		if f.EntityType == entityTypeActivity {
			visErr = auth.EnsureActivityContentVisible(ctx, tx, f.EntityID)
		} else {
			visErr = auth.EnsureVisible(ctx, tx, f.EntityType, f.EntityID)
		}
		if visErr != nil {
			return visErr
		}
		boundary, err := latestScrubTombstone(ctx, tx, f.EntityType, f.EntityID)
		if err != nil {
			return err
		}
		rows, err := queryRecordHistoryWindow(ctx, tx, f, boundary, cursor, useCursor, limit+1)
		if err != nil {
			return err
		}
		if len(rows) > limit {
			rows = rows[:limit]
			next, err := storekit.EncodeCursor(rows[limit-1].occurredAt, rows[limit-1].id)
			if err != nil {
				return err
			}
			page.HasMore = true
			page.NextCursor = next
		}
		for _, row := range rows {
			page.Entries = append(page.Entries, recordHistoryEntry(row, mask))
		}
		return nil
	})
	if err != nil {
		return RecordHistoryPage{}, err
	}
	return page, nil
}

// queryRecordHistoryWindow fetches one keyset window of the record's audit
// spine, newest first, with both display names resolved in SQL by the shared
// auditActorNameJoins — and, unioned into the same window, the rows of the LINKS
// the record is an end of (edgehistory.go).
//
// Neither join carries a workspace predicate, and does not need one: both
// match app_user.id, a global primary key, so a uuid can only ever resolve
// the one person it names. What bounds the ROWS is the caller's gate — the
// row-scope visibility check above for this read, admin-only for the
// compliance log — not anything on the joined table.
func queryRecordHistoryWindow(ctx context.Context, tx pgx.Tx, f RecordHistoryFilter,
	boundary scrubBoundary, cursor storekit.Cursor, useCursor bool, fetch int,
) ([]recordAuditRow, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	typePos, idPos := arg(f.EntityType), arg(f.EntityID)

	var conds []string
	if boundary.exists() {
		// Tombstone-INCLUSIVE (>=), where field-history cuts strictly
		// after (>): projecting a tombstone's payload as field diffs would
		// fabricate changes, so that read withholds the row — but here the
		// tombstone renders as its own honest line ("… erased the
		// record"), and its images are empty since the scrub meta rides
		// evidence. Everything strictly older is still the PII the scrub
		// certified gone and stays withheld, an edge row included: an employment
		// image holds the role and the dates the scrub covered.
		conds = append(conds, fmt.Sprintf("(a.occurred_at, a.id) >= ($%d, $%d)", len(args)+1, len(args)+2))
		args = append(args, boundary.occurredAt, boundary.id)
	}
	if f.Action != nil {
		conds = append(conds, fmt.Sprintf("a.action = $%d", len(args)+1))
		args = append(args, *f.Action)
	}
	if useCursor {
		// The page walks BACKWARDS in time, so the keyset takes what is older
		// than the cursor. A `>` here would page away from the reader.
		conds = append(conds, fmt.Sprintf("(a.occurred_at, a.id) < ($%d, $%d)", len(args)+1, len(args)+2))
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	edgeCTE, err := edgeSubjectCTE(ctx, f.EntityType, idPos, arg)
	// No grant on the edge object: the links are ABSENT, never refused — a
	// refusal would tell the caller the record holds links. The denial lands
	// before edgeSubjectCTE registers an argument (held by
	// TestAnEdgelessCallerRegistersNoEdgeArguments), so no placeholder is unbound.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		edgeCTE = ""
	} else if err != nil {
		return nil, err
	}
	// The limit's placeholder is registered, and the statement rendered, BEFORE
	// the call that spreads args. Go fixes evaluation order for a function's
	// operands only in specific cases, so an arg() that appends inside the same
	// call as `args...` may run after the slice is read — leaving the statement
	// naming one more placeholder than the driver binds.
	fetchPos := arg(fetch)
	window := recordHistoryWindowSQL(typePos, idPos, fetchPos, conds, edgeCTE)
	rows, err := tx.Query(ctx, window, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []recordAuditRow
	for rows.Next() {
		var r recordAuditRow
		if err := scanEdgeAuditRow(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
