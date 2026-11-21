// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The done lane: machine actions that already happened, scoped to this reader.
//
// THE SCOPING PROBLEM IS THE WHOLE FILE. audit_log carries no workspace_id —
// migration 1787320004 dropped the tenant column from the append-only ledgers —
// so an audit row cannot be scoped by itself. It is placed by JOINING the table
// its entity_type names, under that table's own gate, and a row whose type this
// build cannot place is counted rather than shown.
//
// ONE QUERY PER ENTITY TYPE, not one per row. Each type has a different gate and
// a different table, so they cannot share a WHERE — but they can share a window,
// a limit and an order, and the results merge into one page. The alternative,
// one query over audit_log with the scoping done afterwards in Go, would read
// rows the caller may not see in order to discard them, and a bug in the discard
// is a silent disclosure rather than a failed query.
//
// NO PLACEHOLDER IS HAND-TYPED. Every $N is derived from the argument slice, as
// the rulebook asks of any statement built beside its arguments — and both auth
// clauses number themselves from wherever this query's own arguments end, so a
// literal here would be a second place to keep in step.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// entry is one admitted audit row, before it is dressed as a line.
type entry struct {
	ID         ids.UUID
	OccurredAt time.Time
	Action     string
	EntityType string
	EntityID   ids.UUID
	ActorType  string
	ActorID    string
	OnBehalfOf *ids.UUID
	Before     []byte
	After      []byte
	// Label is the record's own name, read through the same join that scopes
	// the row, so a reader is only ever told the name of a record they may see.
	Label *string
	// Evidence is the audit row's own account of why: the policy behind a
	// retention action, the page a fact was read from.
	Evidence []byte
	// BatchID is the bulk change the row belongs to. A data task stamps its
	// step's run id here (datatask.Execute), so two runs of one correction are
	// told apart. Nil on rows no batch wrote.
	BatchID *ids.UUID
	// ActivityKind is what a placed activity is (email, meeting, ...), so a
	// create says "saved an email" rather than "an email or a meeting".
	ActivityKind *string
	// ParentType and ParentID name the record a placed child belongs to
	// (placement.go). Nil on a row about a record with its own page.
	ParentType *string
	ParentID   *ids.UUID
}

// recordLabels is the name column of each placed record, the expression the
// line's About column shows. An activity is named by its subject, read through
// the content clause that already decided the reader may see it.
var recordLabels = map[string]string{
	typeDeal:     "e.name",
	typeCompany:  "e.display_name",
	typeContact:  "e.full_name",
	typeLead:     "e.full_name",
	typeProject:  "e.name",
	typeActivity: "e.subject",
}

// readCap bounds the audit rows one arm reads before lines are grouped. The
// page shows at most maxLimit LINES, but a background job writes one audit row
// per record it touched, and grouping has to see the job's rows to say how many
// there were: cutting at the line limit first counted 100 of 1,200.
const readCap = 5000

// scopedTypes are the entity types this build can place, and how.
//
// The five owner-scoped records go through auth.ScopeClauseFor, which is the
// row-visibility predicate every other read of them uses. `activity` is not in
// this map: its audience is not an owner question, and a row-scope clause would
// miss the narrowing that decides who may read a message — it gets its own arm
// below. `approval` likewise answers to the approvals service.
//
// A type absent from every arm is NOT SHOWN and counted. Serving it would be
// serving a row this read cannot prove the reader may see, and the failure would
// be invisible: the row looks like every other row.
var scopedTypes = map[string]string{
	typeDeal:    typeDeal,
	typeCompany: typeCompany,
	typeContact: typeContact,
	typeLead:    typeLead,
	typeProject: typeProject,
}

// The record types the done lane places, each also the table it joins.
const (
	typeDeal    = "deal"
	typeCompany = "company"
	typeContact = "contact"
	typeLead    = "lead"
	typeProject = "project"
	// typeActivity is placed by its own arm, under the audience gate.
	typeActivity = "activity"
)

// doneSince reads the admitted machine actions in the window, for the records
// this reader may see and works on.
//
// TWO READS PER TYPE. Creates and archives are read apart from everything
// else, each under its own readCap: a mailbox import writes one create per
// contact, and in one read thousands of them would fill the cap and push every
// other change on contacts off the page.
func doneSince(
	ctx context.Context, tx pgx.Tx, since time.Time, limit int,
) ([]entry, map[string]bool, error) {
	width := max(readCap, limit)
	found := make([]entry, 0, limit)
	// An arm that came back full was CUT: readCap is a LIMIT, so the rows it
	// did not return are indistinguishable from rows that do not exist, and
	// every count grouped out of it is a floor rather than a total.
	capped := map[string]bool{}
	for _, bulk := range []bool{false, true} {
		actions := admittedActions(bulk)
		for entityType, table := range scopedTypes {
			rows, err := doneForType(ctx, tx, entityType, table, since, actions, width)
			if err != nil {
				return nil, nil, err
			}
			if len(rows) == width {
				capped[armOf(entityType, bulk)] = true
			}
			found = append(found, rows...)
		}
		activities, err := doneForActivities(ctx, tx, since, actions, width)
		if err != nil {
			return nil, nil, err
		}
		if len(activities) == width {
			capped[armOf(typeActivity, bulk)] = true
		}
		found = append(found, activities...)
		children, err := doneForPlacements(ctx, tx, since, actions, width)
		if err != nil {
			return nil, nil, err
		}
		if len(children) == width {
			capped[armOf(armPlaced, bulk)] = true
		}
		found = append(found, children...)
	}
	return found, capped, nil
}

// notShownSince opens the receipt's not_shown tally with what this build could
// not place, counted rather than guessed at. `update` alone is audited against
// some forty entity types and this build places seven; without the count, a
// receipt showing four lines implies those were the only four machine actions
// in the window, which is the completeness claim the field exists to refuse.
// The breakdown by type is System health's; both need ai_diagnostics read.
func notShownSince(ctx context.Context, tx pgx.Tx, since time.Time) (map[string]int, bool, error) {
	notShown := map[string]int{}
	changes, err := unplacedChangesSince(ctx, tx, since)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return notShown, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	if unplaceable := UnplacedTotal(changes); unplaceable > 0 {
		notShown[string(crmcontracts.MagicNotShownReasonMagicNotShownUnknownEntityType)] = unplaceable
	}
	return notShown, false, nil
}

// armOf names the read an entry came from, which is what a cut is recorded
// against.
func armOf(entityType string, bulk bool) string {
	if bulk {
		return entityType + "/bulk"
	}
	return entityType
}

// armOfEntry names the read one entry came from.
func armOfEntry(e entry) string {
	if e.ParentType != nil {
		return armOf(armPlaced, bulkActions[e.Action])
	}
	return armOf(e.EntityType, bulkActions[e.Action])
}

// notRetention keeps retention's own creates and archives out of the done lane:
// retentionSince reports them as counts, and listing them here as well would
// say twice what retention did.
const notRetention = `NOT (a.action = ANY('{create,archive}'::text[]) AND coalesce(a.evidence, '{}'::jsonb) ? 'retention_action')`

// doneForType reads one owner-scoped entity type's machine actions.
//
// The JOIN is what scopes the row: an audit entry whose record this reader
// cannot see does not survive it, and the predicate is the table's own rather
// than one written here.
func doneForType(
	ctx context.Context, tx pgx.Tx, entityType, table string, since time.Time, actions []string, width int,
) ([]entry, error) {
	// THE GRANT FIRST, and it is not optional even though the clause below often
	// renders nothing. auth.UnboundedFor answers true for a rep on most of these
	// tables — a deal is workspace-readable in this product — so ScopeClauseFor
	// returns an EMPTY predicate, and a read that leaned on the clause alone
	// would serve every row to a seat holding no grant at all. That is the trap
	// of taking a scope clause for a gate: the clause answers "which of them",
	// never "may this seat read any".
	//
	// A refused grant narrows this arm to nothing and leaves the rest of the
	// page alone: a seat that may not read deals has no business seeing what a
	// machine did to one, and its contacts and projects are still its own.
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sinceAt := arg(since)
	actors := arg(MachineActors())
	actionsAt := arg(actions)
	typeAt := arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, table, "e", arg)
	if err != nil {
		return nil, err
	}
	// Done FOR YOU: a deal is readable by every seat, and what a machine did
	// to another team's deal is not this reader's business.
	worksOn, err := auth.WorksOnClause(ctx, table, "e", arg)
	if err != nil {
		return nil, err
	}
	// THE ERASURE BOUNDARY. audit_log is append-only, so a record erased under
	// Art. 17 or anonymized by retention keeps every image written before the
	// scrub — with the subject's real name, email and phone still in it. The
	// record row itself survives too (anonymized IN PLACE, archived rather than
	// deleted), so it still satisfies the scope clause for its original owner.
	// Without this predicate a receipt over a long window republishes exactly
	// what the erasure certified destroyed.
	//
	// privacy.UnscrubbedImageSQL, never a hand-written copy: its own comment
	// asks every reader of this boundary to call it rather than restate it, and
	// says why — almost-the-same is how a certified erasure comes back.
	unscrubbed := privacy.UnscrubbedImageSQL("a", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs())))

	where := fmt.Sprintf(
		`a.occurred_at >= $%d AND a.actor_type = ANY($%d) AND a.action = ANY($%d) AND a.entity_type = $%d AND %s AND %s AND %s`,
		sinceAt, actors, actionsAt, typeAt, unscrubbed, notRetention, worksOn)
	where += completedCutoff(ctx, arg)
	if scope != "" {
		where += " AND " + scope
	}
	// The table name is a compile-time value out of scopedTypes, never a string
	// off a request — the one form of identifier interpolation this tree allows.
	query := fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.action, a.entity_type, a.entity_id,
		       a.actor_type, a.actor_id, a.on_behalf_of, %s,
		       NULLIF(%s, ''), a.evidence, a.batch_id, NULL::text, NULL::text, NULL::uuid
		  FROM audit_log a
		  JOIN %s e ON e.id = a.entity_id
		 WHERE %s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, groupingImages, recordLabels[entityType], table, where, arg(width))
	return scanEntries(ctx, tx, query, args)
}

// doneForActivities is the same read for messages, under the AUDIENCE gate
// rather than the owner one.
//
// Both halves, because they answer different questions and neither implies the
// other: auth.Require is the object grant (may this seat read activities at
// all), and ActivityContentClause is the row predicate (which ones). A clause
// without the grant serves every row to a seat that lost activity.read entirely,
// which is the trap of taking a scope clause for a gate.
func doneForActivities(
	ctx context.Context, tx pgx.Tx, since time.Time, actions []string, width int,
) ([]entry, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// A DENIAL narrows this arm to nothing and leaves the rest of the page
		// alone: a seat that may not read messages sees no message receipts, and
		// its deals and contacts are still its own.
		//
		// Any OTHER error is real and reaches the caller. Swallowing it would
		// report an honest-looking receipt over a read that never ran, which is
		// the shape a missing lane takes when nobody is told it is missing.
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	sinceAt := arg(since)
	actors := arg(MachineActors())
	actionsAt := arg(actions)
	content, err := auth.ActivityContentClause(ctx, "e", arg)
	if err != nil {
		return nil, err
	}
	// A workspace-audience message is readable by every seat; done for you
	// is only what the reader works on.
	worksOn, err := auth.ActivityWorksOnClause(ctx, "e", arg)
	if err != nil {
		return nil, err
	}
	// The same boundary, for the same reason: an erased message's audit images
	// hold what the erasure certified gone.
	unscrubbed := privacy.UnscrubbedImageSQL("a", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs())))
	where := fmt.Sprintf(
		`a.occurred_at >= $%d AND a.actor_type = ANY($%d) AND a.action = ANY($%d) AND a.entity_type = 'activity' AND %s AND %s AND %s`,
		sinceAt, actors, actionsAt, unscrubbed, notRetention, worksOn)
	where += completedCutoff(ctx, arg)
	if content != "" {
		where += " AND " + content
	}
	query := fmt.Sprintf(`
		SELECT a.id, a.occurred_at, a.action, a.entity_type, a.entity_id,
		       a.actor_type, a.actor_id, a.on_behalf_of, %s,
		       NULLIF(%s, ''), a.evidence, a.batch_id, e.kind, NULL::text, NULL::uuid
		  FROM audit_log a
		  JOIN activity e ON e.id = a.entity_id
		 WHERE %s
		 ORDER BY a.occurred_at DESC, a.id DESC
		 LIMIT $%d`, groupingImages, recordLabels[typeActivity], where, arg(width))
	return scanEntries(ctx, tx, query, args)
}

// groupingImages reads an arm row's images only where describe needs them to
// group the row. An update's sentence names the fields it moved. An arm reads
// up to readCap rows, and every other row's images are shown on at most one
// line, so they wait for loadDeferredImages.
var groupingImages = fmt.Sprintf(
	`CASE WHEN a.action = '%[1]s' THEN a.before END, CASE WHEN a.action = '%[1]s' THEN a.after END`, actionUpdate)

// imagesDeferred reports a row whose images groupingImages left unread and a
// line still shows. A bulk verb's images are never shown, nor a placed child's.
func imagesDeferred(e entry) bool {
	return e.Action != actionUpdate && !bulkActions[e.Action] && e.ParentType == nil
}

// loadDeferredImages reads the images the arms left unread, for the entries a
// page shows, in one statement, inside the snapshot the scoped arms read. The
// erasure boundary and a message's hold are asked again all the same. A row it
// no longer serves keeps no images.
func loadDeferredImages(ctx context.Context, tx pgx.Tx, entries []entry) error {
	at := map[ids.UUID]int{}
	for i, e := range entries {
		if imagesDeferred(e) {
			at[e.ID] = i
		}
	}
	if len(at) == 0 {
		return nil
	}
	auditIDs := make([]ids.UUID, 0, len(at))
	for id := range at {
		auditIDs = append(auditIDs, id)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	// A message put on hold since the arm read it is withheld like an erased one.
	query := fmt.Sprintf(`
		SELECT entry.id, entry.before, entry.after
		  FROM audit_log entry
		 WHERE entry.id = ANY($%d) AND %s
		   AND (entry.entity_type <> 'activity' OR EXISTS (
		         SELECT 1 FROM activity held WHERE held.id = entry.entity_id AND %s))`,
		arg(auditIDs), privacy.UnscrubbedImageSQL("entry", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs()))),
		auth.ActivityExternalEvidenceClause("held"))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("read the shown machine actions' images: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var before, after []byte
		if err := rows.Scan(&id, &before, &after); err != nil {
			return fmt.Errorf("read a shown machine action's images: %w", err)
		}
		entries[at[id]].Before, entries[at[id]].After = before, after
	}
	return rows.Err()
}

// scanEntries reads the rows one arm returned.
func scanEntries(ctx context.Context, tx pgx.Tx, query string, args []any) ([]entry, error) {
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read the machine actions: %w", err)
	}
	defer rows.Close()
	found := []entry{}
	for rows.Next() {
		var e entry
		if err := rows.Scan(
			&e.ID, &e.OccurredAt, &e.Action, &e.EntityType, &e.EntityID,
			&e.ActorType, &e.ActorID, &e.OnBehalfOf, &e.Before, &e.After,
			&e.Label, &e.Evidence, &e.BatchID, &e.ActivityKind, &e.ParentType, &e.ParentID,
		); err != nil {
			return nil, fmt.Errorf("read a machine action: %w", err)
		}
		found = append(found, e)
	}
	return found, rows.Err()
}

// admittedActions lists the actions one read shows, for the SQL filter: the
// bulk verbs, or every other admitted one.
//
// Filtering in the query rather than in Go is what keeps the count honest: a
// read that fetched everything and dropped the rest would page over rows it
// never shows, so a page of twenty could come back holding three.
func admittedActions(bulk bool) []string {
	actions := make([]string, 0, len(admitted))
	for action := range admitted {
		if bulkActions[action] == bulk {
			actions = append(actions, action)
		}
	}
	return actions
}
