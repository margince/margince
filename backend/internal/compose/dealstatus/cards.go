// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// The cached standing, read for a page of deals at once.
//
// CachedMoves beside this one answers what to DO about a deal. This answers how
// the deal is standing — the judgement the deal page prints above that move, and
// the half a queue row was missing. A row naming a step without saying whether
// the deal is alive sends the reader to the deal page to find out, which is the
// hand-off the queue exists to remove.
//
// EVERY RULE CachedMoves STATES APPLIES HERE UNCHANGED, and for the same
// reasons: it reads the cache and never assembles, it is keyed per reader, it
// accepts staleness, and a deal with no card contributes nothing. The two are
// separate functions rather than one because they are read at different moments
// — figures and moves are gathered after the page is cut, and a caller wanting
// only the standing should not pay for the audience re-gate a move needs.
//
// NO AUDIENCE RE-GATE HERE, deliberately, and the asymmetry is the point. A move
// names a RECORD the reader may have lost access to, so serving it would say a
// message exists. A standing is a sentence about the deal, written from records
// admitted when the card was written, and the reader holds `deal.read` or this
// row would not have reached them. What the sentence CITES is not carried out —
// only its text — so there is no id here to serve past a gate.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CachedCard is one deal's already-written standing, as much of it as a queue
// row draws.
type CachedCard struct {
	// Standing is the card's own verdict word: live, drifting, blocked, cold.
	Standing string
	// DecisiveLine is the first sentence of what that verdict rests on.
	DecisiveLine string
	// GeneratedAt is when the card behind this reading was written.
	GeneratedAt time.Time
}

// CachedCards answers the already-written standing for each of these deals, for
// the reader this context authenticates.
//
// A deal is absent from the answer when no card is cached for it, when its
// stored payload this build cannot read, or when the card carries no verdict —
// the three mean one thing to a caller, which is that there is no standing to
// show here.
func (s *Service) CachedCards(
	ctx context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]CachedCard, error) {
	out := make(map[ids.UUID]CachedCard, len(dealIDs))
	if len(dealIDs) == 0 {
		return out, nil
	}
	userID, err := actingUser(ctx)
	if err != nil {
		return nil, err
	}
	err = database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT deal_id, payload, generated_at FROM deal_status_card
			WHERE user_id = $1 AND deal_id = ANY($2)`,
			userID, dealIDs)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dealID ids.UUID
			var payload []byte
			var generatedAt time.Time
			if err := rows.Scan(&dealID, &payload, &generatedAt); err != nil {
				return err
			}
			card, ok := cardFromPayload(payload)
			if !ok {
				continue
			}
			card.GeneratedAt = generatedAt
			out[dealID] = card
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("read the cached deal standings: %w", err)
	}
	return out, nil
}

// cardFromPayload lifts one card's verdict out of its stored blob.
//
// An unreadable payload is a MISS rather than a failure, for the reason
// moveFromPayload gives: failing a whole page of the queue over one stale blob
// would take every other row's reading away with it.
//
// A verdict whose `because` has no sentences is dropped WHOLE. The standing word
// alone would be a judgement with nothing behind it — "this deal is blocked" and
// no way to ask why — and the queue's own deterministic reason, which the caller
// falls back to, at least names the fact it rests on.
func cardFromPayload(payload []byte) (CachedCard, bool) {
	var stored stored
	if err := json.Unmarshal(payload, &stored); err != nil {
		return CachedCard{}, false
	}
	verdict := stored.Card.Verdict
	if verdict == nil || verdict.Standing == "" {
		return CachedCard{}, false
	}
	line, ok := firstSentence(verdict.Because)
	if !ok {
		return CachedCard{}, false
	}
	return CachedCard{Standing: verdict.Standing, DecisiveLine: line}, true
}

// firstSentence takes the one line a row has room for out of a cited section.
//
// The FIRST, not a join of all of them: the card writes its sentences in the
// order it wants them read, and a queue row draws one line. Concatenating them
// would compose a sentence nobody wrote, in a length the row cannot draw.
func firstSentence(section crmcontracts.DealStatusCardSection) (string, bool) {
	for _, sentence := range section.Sentences {
		if sentence.Text != "" {
			return sentence.Text, true
		}
	}
	return "", false
}
