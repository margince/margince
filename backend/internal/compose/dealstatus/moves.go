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
		for dealID, move := range found {
			named, ok := NamedActivity(move)
			if ok && !readable[named] {
				// The reader has lost the message this move is about. Dropping
				// the move WHOLE is the point: serving it without its operand
				// would still say a message exists and is unanswered.
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

// NamedActivity reads the record a verb acts on out of a move's arguments.
//
// Nothing rather than a zero id for a value this cannot parse, and nothing for a
// verb that names no record at all — a create_task or an opening outreach acts
// on no existing row, so there is nothing here for the audience gate to judge.
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
	raw, present := (*args)["activity_id"]
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
