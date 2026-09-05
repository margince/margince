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
// AND THE CITED ACTIVITIES ARE RE-GATED ON EVERY READ, exactly as CachedMoves
// re-gates the record its move names. The first version of this file argued the
// opposite — that a standing carries only prose and names no record, so there
// was nothing to gate — and that argument is wrong in the way that matters. The
// sentence is MODEL-WRITTEN FROM the timeline and grounding.go requires it to
// cite the records it rests on, so its text restates content from those
// records. Auth's rule is about content derived from an activity, not about ids:
// everything so derived carries the audience predicate wherever it is served.
//
// The deal page does not have this problem, which is why it needed no such
// gate. Service.Get re-gathers the timeline under the caller's CURRENT grants
// and fingerprints the card against it, so a mail narrowed after the card was
// written changes the input, misses the fingerprint, and the card is rewritten
// without it. This read has no fingerprint to miss — that is the whole of what
// makes it cheap — so it asks the audience question directly instead.
//
// A standing whose cited activity this reader may no longer read is dropped
// WHOLE, the same rule and for the same reason as a move's: the row then falls
// through to its typed deterministic reasons, which explain it without any
// model in the path.

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
	// CitedActivities are the messages that sentence was written from, which the
	// caller re-gates before serving it. Empty where the sentence cites only
	// records that are not activities — a deal's own fields carry no audience.
	CitedActivities []ids.UUID
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
		found, err := readCachedCards(ctx, tx, userID, dealIDs)
		if err != nil {
			return err
		}
		// The audience, asked again NOW rather than trusted from write time.
		// One query for the whole page, over every activity any of these
		// sentences cites — the same shape and the same reader CachedMoves uses.
		readable, err := readableActivities(ctx, tx, citedActivities(found))
		if err != nil {
			return err
		}
		for dealID, card := range found {
			if !allReadable(card.CitedActivities, readable) {
				// The reader has lost a message this sentence was written from.
				// Dropping the standing WHOLE is the point: its text restates
				// what that message said.
				continue
			}
			out[dealID] = card
		}
		return nil
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
	sentence, ok := firstSentence(verdict.Because)
	if !ok {
		return CachedCard{}, false
	}
	return CachedCard{
		Standing:        verdict.Standing,
		DecisiveLine:    sentence.Text,
		CitedActivities: activitiesCitedBy(sentence),
	}, true
}

// activitiesCitedBy reads the messages one sentence was written from.
//
// ACTIVITIES ONLY, because they are the records that carry an audience. A
// sentence citing a deal or a person rests on rows the reader already reached —
// the deal grant is what put this row on their queue — and asking the audience
// question about a record that has none would refuse every standing.
func activitiesCitedBy(sentence crmcontracts.OrganizationBriefSentence) []ids.UUID {
	cited := make([]ids.UUID, 0, len(sentence.Evidence))
	for _, evidence := range sentence.Evidence {
		if evidence.EntityType != crmcontracts.OrganizationBriefEvidenceEntityTypeActivity {
			continue
		}
		cited = append(cited, ids.UUID(evidence.EntityId))
	}
	return cited
}

// firstSentence takes the one line a row has room for out of a cited section.
//
// The FIRST, not a join of all of them: the card writes its sentences in the
// order it wants them read, and a queue row draws one line. Concatenating them
// would compose a sentence nobody wrote, in a length the row cannot draw.
func firstSentence(
	section crmcontracts.DealStatusCardSection,
) (crmcontracts.OrganizationBriefSentence, bool) {
	for _, sentence := range section.Sentences {
		if sentence.Text != "" {
			return sentence, true
		}
	}
	return crmcontracts.OrganizationBriefSentence{}, false
}

// readCachedCards takes the stored cards for these deals, for this reader.
//
// Collected whole before anything else runs, for the reason readCachedMoves
// states: the audience filter is a second query on the same transaction, and
// pgx will not start one while these rows are still open.
func readCachedCards(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, dealIDs []ids.UUID,
) (map[ids.UUID]CachedCard, error) {
	found := map[ids.UUID]CachedCard{}
	rows, err := tx.Query(ctx, `
		SELECT deal_id, payload, generated_at FROM deal_status_card
		WHERE user_id = $1 AND deal_id = ANY($2)`,
		userID, dealIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var dealID ids.UUID
		var payload []byte
		var generatedAt time.Time
		if err := rows.Scan(&dealID, &payload, &generatedAt); err != nil {
			return nil, err
		}
		card, ok := cardFromPayload(payload)
		if !ok {
			continue
		}
		card.GeneratedAt = generatedAt
		found[dealID] = card
	}
	return found, rows.Err()
}

// citedActivities collects the messages these standings were written from,
// deduplicated, so the audience question is asked once per record rather than
// once per deal.
func citedActivities(cards map[ids.UUID]CachedCard) []ids.UUID {
	seen := map[ids.UUID]bool{}
	wanted := make([]ids.UUID, 0, len(cards))
	for _, card := range cards {
		for _, id := range card.CitedActivities {
			if seen[id] {
				continue
			}
			seen[id] = true
			wanted = append(wanted, id)
		}
	}
	return wanted
}

// allReadable answers whether the reader may still read every message this
// sentence was written from.
//
// EVERY one, not any: a sentence rests on all of its citations at once, and
// serving it because one of three survived would still restate what the other
// two said.
func allReadable(cited []ids.UUID, readable map[ids.UUID]bool) bool {
	for _, id := range cited {
		if !readable[id] {
			return false
		}
	}
	return true
}
