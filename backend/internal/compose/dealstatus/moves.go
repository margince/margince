// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The cached moves, read for a page of deals at once.
//
// The queue needed what this package already decides. A row saying a deal has
// been quiet for ninety days names the problem and stops; the deal's own page,
// standing on the same records, says which mail to answer or which meeting to
// prepare. Two surfaces over one deal, and only one of them told the reader
// what to do.
//
// READS THE CACHE, NEVER ASSEMBLES. Get() gathers a timeline, seats and a deal
// room per card, and may call a model — costs a page of thirty rows cannot pay
// thirty times over. So this reads what is already written and nothing else: a
// deal with no cached card contributes no move, and its row says what it said
// before this existed. That is the N+1 the attention feed exists to avoid, and
// the rule dealfacts.go and labels.go state on the other side.
//
// STALENESS IS ACCEPTED, DELIBERATELY. The card's own read compares the stored
// fingerprint against the facts and rewrites a card the records moved under.
// This does not — recomputing the fingerprint means gathering the facts, which
// is the assembly being avoided. So a move can name a message somebody has
// since answered. On the deal page that would be wrong; on a queue row it is a
// suggestion the reader is about to look at anyway, and the alternative is no
// suggestion at all.
//
// PER READER, like every other read of this table. The key is (user_id,
// deal_id) because a card is written from records its reader may see, and a
// move naming a message one rep may read is not a move to hand another.
//
// AND THE ACTIVITY IS RE-GATED ON EVERY READ, which the reader key alone does
// NOT do. The cache preserves an admission taken when the card was WRITTEN, and
// what admitted it is a thing a human changes afterwards — two things, in fact,
// and losing either one is enough:
//
//   - the OBJECT grant. A seat can lose `activity.read` and keep `deal.read`.
//   - the AUDIENCE. A rep reading a mail as a workspace member is outside it the
//     moment somebody narrows that mail to its participants.
//
// Either way the deal still reaches their queue, because the at-risk lane asks
// for the deal grant and nothing more. Serving the stored activity id there
// tells them the message exists and is unanswered — content derived from an
// activity, and auth's own rule is that everything so derived carries the
// audience predicate wherever it is served.
//
// So both are asked again on every read, and a move whose record this reader may
// no longer read is dropped WHOLE rather than served without its operand.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CachedMoves answers the already-written move for each of these deals, for the
// reader this context authenticates.
//
// A deal is absent from the answer when no card is cached for it, when its
// stored payload this build cannot read, when the card's own answer was that
// there is no next step, or when the move names a record this reader may no
// longer read. The four cases mean one thing to a caller, which is that there
// is nothing to suggest here.
func (s *Service) CachedMoves(
	ctx context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	out := make(map[ids.UUID]crmcontracts.DealStatusCardMove, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	// The same admission CachedCards states. This reader shares that cache and
	// asked the object question no more than it did.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return nil, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		found, err := readCachedMoves(ctx, tx, userID, dealIDs)
		if err != nil {
			return err
		}
		// The audience, asked again NOW rather than trusted from write time.
		// One query for the whole page, and it runs only where a move actually
		// names a record — a page of create_task moves asks nothing.
		readable, err := readableActivities(ctx, tx, namedActivities(found))
		if err != nil {
			return err
		}
		// The contacts a move would file work against, judged the same way and
		// in the same transaction. A move naming a contact is as much a
		// disclosure as one naming a message, and the card outlives the grant
		// that allowed it: without this, a reader who loses a contact keeps
		// being told to book a meeting with them, by name, from the queue.
		contactsFileable, err := contactsStillFileable(ctx, tx, contactsAcrossMoves(found))
		if err != nil {
			return err
		}
		for dealID, move := range found {
			named, ok := NamedActivity(move)
			if ok && !readable[named] {
				// The reader has lost the message this move is about. Dropping
				// the move WHOLE is the point: serving it without its operand
				// would still say a message exists and is unanswered.
				continue
			}
			if !everyNamedContactStillFileable(move, contactsFileable) {
				continue
			}
			out[dealID] = move
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the cached deal moves: %w", err)
	}
	return out, nil
}

// readCachedMoves takes the stored cards for these deals, for this reader.
//
// Collected whole before anything else runs: the audience filter below is a
// second query on the same transaction, and pgx will not start one while these
// rows are still open.
func readCachedMoves(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, dealIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	found := map[ids.UUID]crmcontracts.DealStatusCardMove{}
	rows, err := tx.Query(ctx, `
		SELECT deal_id, payload FROM deal_status_card
		WHERE user_id = $1 AND deal_id = ANY($2)`,
		userID, dealIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dealID ids.UUID
		var payload []byte
		if err := rows.Scan(&dealID, &payload); err != nil {
			return nil, err
		}
		move, ok := moveFromPayload(payload)
		if !ok {
			continue
		}
		found[dealID] = move
	}
	return found, rows.Err()
}

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
	raw, present := (*args)["links"]
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
	switch id := raw.(type) {
	case ids.UUID:
		return id, true
	case openapi_types.UUID:
		return ids.UUID(id), true
	case string:
		parsed, err := ids.Parse(id)
		if err != nil {
			return ids.UUID{}, false
		}
		return parsed, true
	default:
		return ids.UUID{}, false
	}
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

// NamedActivity reads the record a verb acts on out of a move's arguments.
//
// Nothing rather than a zero id for a value this cannot parse, and nothing for a
// verb that names no record at all. Request acceptance acts on its source
// message; a generic task or opening outreach has no source to judge.
//
// EXPORTED because the queue reads the same key when it lifts the id onto the
// wire, and the two answers must agree: an id the wire names but this did not
// judge is an id served past the audience gate below. The queue cannot call
// this directly — it is a sibling compose package, and that import is the edge
// the seam interface exists to avoid — so it keeps its own copy, and
// backend/gates/dealmoveargument_test.go holds the two to one answer.
func NamedActivity(move crmcontracts.DealStatusCardMove) (ids.UUID, bool) {
	args := move.Arguments
	if args == nil {
		return ids.UUID{}, false
	}
	raw, present := (*args)["request_activity_id"]
	if !present {
		raw, present = (*args)["activity_id"]
	}
	if !present {
		return ids.UUID{}, false
	}
	text, ok := raw.(string)
	if !ok {
		return ids.UUID{}, false
	}
	id, err := ids.Parse(text)
	if err != nil {
		return ids.UUID{}, false
	}
	return id, true
}

// moveFromPayload lifts one card's move out of its stored blob.
//
// An unreadable payload is a MISS rather than a failure, the same way cached()
// treats one: the card is derived content the deal page rewrites on its next
// read, and failing a whole page of the queue over one stale blob would take
// every other row's move away with it.
//
// A `none` move is dropped here rather than carried out. It is a real answer on
// the deal page — the card says in words that a closed deal has no next step —
// but a queue row has no sentence to put it in, and a caller made to recognize
// it would be a second place spelling the same rule.
func moveFromPayload(payload []byte) (crmcontracts.DealStatusCardMove, bool) {
	var card stored
	if err := json.Unmarshal(payload, &card); err != nil {
		return crmcontracts.DealStatusCardMove{}, false
	}
	move := card.Card.Next
	if move == nil || move.Action == "" || move.Action == ActionNone {
		return crmcontracts.DealStatusCardMove{}, false
	}
	return *move, true
}
