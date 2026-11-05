// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// Access checks shared by current moves and dated assessments.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// namedActivities collects the records these moves point at, deduplicated, so
// the audience question below is asked once per record rather than once per
// deal.
func namedActivities(moves map[ids.UUID]crmcontracts.DealStatusCardMove) []ids.UUID {
	seen := map[ids.UUID]bool{}
	wanted := make([]ids.UUID, 0, len(moves))
	for _, move := range moves {
		id, ok := NamedActivity(move)
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
	}
	return wanted
}

// readableActivities answers which of these records the caller may read RIGHT
// NOW — granted the object, discoverable, and inside the activity's audience.
//
// BOTH HALVES, because they answer different questions and neither implies the
// other. auth.Require is the OBJECT grant: may this seat read activities at all.
// auth.ActivityContentClause is a ROW predicate: which ones, and its audience
// arm is what a discover-only clause would miss. A clause without the grant
// would serve every row to a seat that lost `activity.read` entirely, which is
// the trap of taking a scope clause for a gate.
//
// A refused GRANT answers no readable records rather than failing the page: a
// seat that may not read activities has no business seeing a move that names
// one, and the rest of the page — its deals, its figures, its record-free
// moves — is theirs to see. So the refusal narrows this read and stops there.
//
// An id absent from the answer is one this reader may not read: no grant, the
// row is gone, or somebody narrowed its audience after the card was written.
func readableActivities(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	readable := map[ids.UUID]bool{}
	if len(activityIDs) == 0 {
		return readable, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return readable, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	ids0 := arg(activityIDs)
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	where := fmt.Sprintf("a.id = ANY($%d)", ids0)
	if content != "" {
		where += " AND " + content
	}
	rows, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		readable[id] = true
	}
	return readable, rows.Err()
}

// everyNamedContactStillFileable reports whether this reader may still file work
// against every contact the move names. One unreadable contact drops the move whole, for the reason
// the activity arm above drops one: a move stripped of the contact it is about
// still says somebody is there.
func everyNamedContactStillFileable(move crmcontracts.DealStatusCardMove, fileable map[ids.UUID]bool) bool {
	for _, id := range NamedContacts(move) {
		if !fileable[id] {
			return false
		}
	}
	return true
}

// NamedContacts reads the contacts a move would file work against, out of the
// links its arguments carry.
//
// A move that says "book a meeting with Annabelle Malherbe" names a contact in
// its subject AND links the task to them. Both are a disclosure: the move is
// stored per reader and served again later, so a reader who has since lost that
// contact must not be handed either. The activity ids beside it are judged the
// same way and for the same reason — see readableActivities.
//
// A malformed link is skipped rather than failing the page: the queue drops a
// move it cannot read whole, which is the existing posture for a stale payload.
//
// IT READS BOTH SHAPES OF THE SAME MOVE, and that is the whole difficulty. A
// move decideMove has just built carries Go values — []map[string]any, a typed
// UUID — while the same move read back out of the cache has been through JSON
// and carries []any and strings. A reader that knew only the stored shape
// returned nothing for every fresh move, which is silent: the fingerprint then
// misses the operand and the audience check passes because it found nobody to
// check.
func NamedContacts(move crmcontracts.DealStatusCardMove) []ids.UUID {
	args := move.Arguments
	if args == nil {
		return nil
	}
	raw, present := (*args)[argLinks]
	if !present {
		return nil
	}
	var out []ids.UUID
	for _, link := range linkMaps(raw) {
		if kind, ok := link[argEntityType].(string); !ok || kind != linkContact {
			continue
		}
		if id, ok := linkID(link[argEntityID]); ok {
			out = append(out, id)
		}
	}
	return out
}

// linkMaps lifts a move's links whichever of the two shapes it is in.
//
// The argument IS an untyped value: one arm of the contract's `arguments`
// object, which is `additionalProperties: true` and arrives as whatever JSON
// left behind. Naming a type would be naming a shape this cannot rely on.
func linkMaps(raw any) []map[string]any { //craft:ignore naked-any reads the contract's open arguments object
	switch links := raw.(type) {
	case []map[string]any:
		return links
	case []any:
		out := make([]map[string]any, 0, len(links))
		for _, entry := range links {
			if link, ok := entry.(map[string]any); ok {
				out = append(out, link)
			}
		}
		return out
	default:
		return nil
	}
}

// linkID reads a link's target id, as a typed value on a fresh move or as the
// string JSON left behind on a cached one.
//
// Same reason as linkMaps: one field out of the contract's open `arguments`
// object, where the value's type is exactly what is in question.
func linkID(raw any) (ids.UUID, bool) { //craft:ignore naked-any reads the contract's open arguments object
	return owedwork.RecordArgument(raw)
}

// readableContacts answers which of these contacts the caller may still FILE
// WORK against right now.
//
// It asks the attach predicate rather than the read one, and the difference is
// the point. A cached move carrying a contact link is a task body the reader is
// one click from sending, and a share that has dropped from write to read-only
// since the card was written leaves that body pointing at a record the server
// will refuse (auth.EnsureAttachTarget). Judging it by readability alone would
// serve a button whose only outcome is a permission error, and the reader would
// read that as the product being broken.
//
// The stricter question also answers the weaker one: a reader who may file
// against a contact may read them. So a move that survives this is one whose
// name the reader may see and whose link the reader may write.
//
// Two halves, like readableActivities: the object grant, then the row
// predicate. A refused grant answers none rather than failing the page.
func contactsStillFileable(
	ctx context.Context, tx pgx.Tx, contactIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	fileable := map[ids.UUID]bool{}
	if len(contactIDs) == 0 {
		return fileable, nil
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return fileable, nil
		}
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(contactIDs)
	scope, err := auth.AttachClauseFor(ctx, "contact", "c", arg)
	if err != nil {
		return nil, err
	}
	where := fmt.Sprintf("c.id = ANY($%d) AND c.archived_at IS NULL", idsPos)
	if scope != "" {
		where += " AND " + scope
	}
	rows, err := tx.Query(ctx, `SELECT c.id FROM contact c WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		fileable[id] = true
	}
	return fileable, rows.Err()
}

// contactsAcrossMoves collects every contact named across a page of moves, so
// the audience is asked once for the page rather than once per move.
func contactsAcrossMoves(moves map[ids.UUID]crmcontracts.DealStatusCardMove) []ids.UUID {
	seen := map[ids.UUID]bool{}
	wanted := make([]ids.UUID, 0, len(moves))
	for _, move := range moves {
		for _, id := range NamedContacts(move) {
			if seen[id] {
				continue
			}
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	return wanted
}

// NamedActivity shares the argument parser with the Worklist's wire projection.
func NamedActivity(move crmcontracts.DealStatusCardMove) (ids.UUID, bool) {
	if move.Arguments == nil {
		return ids.UUID{}, false
	}
	return owedwork.ActivityArgument(*move.Arguments)
}
