// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"
	"strings"
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
)

// auditActorNameJoins resolves the two display names every audit read owes
// its reader: the human who acted, and the human whose authority a machine
// acted under. It is ONE spelling shared by the two audit read paths in this
// package — the per-record history here and the workspace-wide compliance log
// in auditlog.go — because two surfaces resolving attribution differently is
// how a reader ends up trusting one and doubting the other.
//
// The audit row is aliased `a`; the caller supplies that alias and selects
// `actor_user.display_name, obo.display_name` in that order.
//
// RecordHistoryFilter carries the validated query surface of
// (GET /records/{entity_type}/{id}/history).
type RecordHistoryFilter struct {
	EntityType string
	EntityID   ids.UUID
	Cursor     *string
	Limit      *int
}

// RecordHistoryEntry is one audit_log row rendered as a history line —
// the whole-mutation view, one entry per row (field-history's per-field
// projection is the sibling read). Before/After are the row's own field
// images with the entity's mask applied by omission. Scrub tombstones and
// retention rows carry their operational tallies on audit_log.evidence,
// which this read never selects; other meta verbs' before/after payloads
// are served verbatim—workspace-operational context behind the same
// record-read gate, never subject PII.
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
	return RecordHistoryEntry{
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
		Before:            applyFieldMask(row.before, mask),
		After:             applyFieldMask(row.after, mask),
		Summary: composeRecordSummary(row.actorType, display, row.onBehalfOfName,
			row.action, row.passportID != nil, row.agentClientName),
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
			page.HasMore = true
			page.NextCursor = storekit.EncodeCursor(rows[limit-1].occurredAt, rows[limit-1].id)
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
// auditActorNameJoins.
//
// Neither join carries a workspace predicate, and does not need one: both
// match app_user.id, a global primary key, so a uuid can only ever resolve
// the one person it names. What bounds the ROWS is the caller's gate — the
// row-scope visibility check above for this read, admin-only for the
// compliance log — not anything on the joined table.
func queryRecordHistoryWindow(ctx context.Context, tx pgx.Tx, f RecordHistoryFilter,
	boundary scrubBoundary, cursor storekit.Cursor, useCursor bool, fetch int,
) ([]recordAuditRow, error) {
	conds := []string{"a.entity_type = $1", "a.entity_id = $2"}
	args := []any{f.EntityType, f.EntityID}
	if boundary.exists() {
		// Tombstone-INCLUSIVE (>=), where field-history cuts strictly
		// after (>): projecting a tombstone's payload as field diffs would
		// fabricate changes, so that read withholds the row — but here the
		// tombstone renders as its own honest line ("… erased the
		// record"), and its images are empty since the scrub meta rides
		// evidence. Everything strictly older is still the PII the scrub
		// certified gone, and stays withheld.
		conds = append(conds, fmt.Sprintf("(a.occurred_at, a.id) >= ($%d, $%d)", len(args)+1, len(args)+2))
		args = append(args, boundary.occurredAt, boundary.id)
	}
	if useCursor {
		// The page walks BACKWARDS in time, so the keyset takes what is older
		// than the cursor. A `>` here would page away from the reader.
		conds = append(conds, fmt.Sprintf("(a.occurred_at, a.id) < ($%d, $%d)", len(args)+1, len(args)+2))
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	args = append(args, fetch)
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT`+recordAuditColumns+`
		FROM audit_log a`+auditActorNameJoins+agentClientNameJoin+`
		WHERE %s
		ORDER BY a.occurred_at DESC, a.id DESC
		LIMIT $%d`, strings.Join(conds, " AND "), len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []recordAuditRow
	for rows.Next() {
		var r recordAuditRow
		if err := scanRecordAuditRow(rows, &r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// machineQualifier names the tool a delegated change was typed through, as
// the phrase that qualifies the PERSON who authorized it — "via an agent",
// not "an agent".
//
// It is the FALLBACK now. When the passport came from an OAuth grant the line
// names the client instead — "Demo Admin, via Claude, created the record" —
// because "via an agent" on every row of a company's history tells a rep
// nothing they did not already know, and the question they actually have is
// which tool did it.
//
// The name comes from oauth_client.client_name, not from passport.label. The
// concern the generic word was protecting against — a revoked passport's label
// outliving the grant on every row it ever wrote — does not apply: client_name
// is the registered identity of the tool, it does not change when a grant is
// revoked, and a client row that is gone falls back to this map.
var machineQualifier = map[string]string{
	actorTypeAgent:     "via an agent",
	actorTypeConnector: "via a connector",
}

// composeRecordSummary renders one audit row as a plain-language sentence,
// the record-history read's `summary` field. It is pure: callers resolve
// actorDisplayName/onBehalfOfName (app_user lookups) before calling in, so
// this stays testable without a database. onBehalfOfName is set only for a
// machine acting under a human's delegated authority (D2's authority
// weaving); an empty string is treated the same as nil — a resolved-but-
// blank name is not authority to report.
//
// The sentence NAMES THE PERSON FIRST and says a machine did the typing
// second (PD-002). A rep working through a passport is the rep: the line
// reads "Devin, via an agent, archived the record", never "an agent archived
// the record" with the person demoted to a trailing phrase. Attribution
// exists so somebody can be asked about a change, and a machine is not a
// party to anything — a line whose subject is the tool lets every human in
// the chain disclaim it.
func composeRecordSummary(actorType, actorDisplayName string, onBehalfOfName *string,
	action string, passportBacked bool, agentClientName *string,
) string {
	verb := recordHistoryVerbs[action]
	if verb == "" {
		verb = action
	}
	if qualifier, delegated := machineQualifier[actorType]; delegated && onBehalfOfName != nil && *onBehalfOfName != "" {
		if agentClientName != nil && *agentClientName != "" {
			qualifier = "via " + *agentClientName
		}
		return fmt.Sprintf("%s, %s, %s the record", *onBehalfOfName, qualifier, verb)
	}
	switch {
	case actorType == actorTypeHuman:
		return fmt.Sprintf("%s %s the record", actorDisplayName, verb)
	case passportBacked:
		// A PASSPORT was presented and yet no human resolved behind it. That
		// is a gap: passport.on_behalf_of is NOT NULL, so the authority
		// existed when the grant was made and the row failed to carry it (the
		// pre-0260 scheduled sends in compose/commsscheduled.go are the live
		// example). The line says so rather than falling back to "System",
		// which is reserved for a change that genuinely has nobody behind it —
		// letting system absorb a failed attribution would hide the gap on the
		// one surface that exists to expose it.
		return fmt.Sprintf("A machine with no recorded human authority %s the record", verb)
	case actorType == actorTypeSystem:
		return fmt.Sprintf("System %s the record", verb)
	case actorType == actorTypeAgent:
		// A background writer with no passport: an installation-wide pass that
		// nobody's context ran, so there is no human to name and no gap to
		// report. compose/extjobsrun.go writes one per extension job tick.
		// Calling this a missing authority would report a defect where there
		// is none — the distinction that matters is whether a grant was
		// presented, not which machine word the actor_type happens to carry.
		return fmt.Sprintf("Agent %s the record", verb)
	case actorType == actorTypeConnector:
		// Same reasoning as the bare agent above: some connectors have no
		// connect flow and therefore no granting human by design
		// (compose/jobs_finance.go writes one).
		return fmt.Sprintf("Connector %s the record", verb)
	default:
		return fmt.Sprintf("%s %s the record", actorType, verb)
	}
}
