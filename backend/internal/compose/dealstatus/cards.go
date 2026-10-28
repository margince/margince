// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

// Cached standings are dated earlier assessments, never current action authority.
// Each read checks the parent deal and the context used to write the assessment.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CachedCard is one deal's already-written standing, as much of it as a queue
// row draws.
type CachedCard struct {
	access *cardAccess
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
	Citations       []crmcontracts.CompanyBriefEvidence
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
	// The same admission Get states. This is the cache-only reader — it never
	// gathers, so nothing else here asks the object question at all.
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
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
		// Citations alone do not cover every input used to write the assessment.
		readable, err := readableActivities(ctx, tx, citedActivities(found))
		if err != nil {
			return err
		}
		masks, err := s.assessmentMasks(ctx, tx, found)
		if err != nil {
			return err
		}
		seats, err := s.readableAssessmentSeats(ctx, tx, found)
		if err != nil {
			return err
		}
		for dealID, card := range found {
			if card.access == nil || (card.access.Room && !auth.ReadGranted(ctx, "deal_room")) ||
				!allReadable(card.CitedActivities, readable) || !allReadable(card.access.Activities, readable) ||
				!masks[dealID] || !allReadable(card.access.Contacts, seats[dealID].named) || !allReadable(card.access.Seats, seats[dealID].visible) {
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

// cardFromPayload omits unsupported assessments and verdicts without cited reasoning.
func cardFromPayload(payload []byte) (CachedCard, bool) {
	var stored stored
	if !decodeStored(payload, &stored) {
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
		access:          stored.Access,
		Standing:        verdict.Standing,
		DecisiveLine:    sentence.Text,
		CitedActivities: activitiesCitedBy(sentence),
		Citations:       sentence.Evidence,
	}, true
}

// activitiesCitedBy reads the messages one sentence was written from.
//
// ACTIVITIES ONLY, because they are the records that carry an audience. A
// sentence citing a deal or a contact rests on rows the reader already reached —
// the deal grant is what put this row on their queue — and asking the audience
// question about a record that has none would refuse every standing.
func activitiesCitedBy(sentence crmcontracts.CompanyBriefSentence) []ids.UUID {
	cited := make([]ids.UUID, 0, len(sentence.Evidence))
	for _, evidence := range sentence.Evidence {
		if evidence.EntityType != crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
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
) (crmcontracts.CompanyBriefSentence, bool) {
	for _, sentence := range section.Sentences {
		if sentence.Text != "" {
			return sentence, true
		}
	}
	return crmcontracts.CompanyBriefSentence{}, false
}

// readCachedCards takes the stored cards for these deals, for this reader.
//
// Collect rows before checking citation audiences on the same transaction;
// pgx cannot query while the first result remains open.
func readCachedCards(
	ctx context.Context, tx pgx.Tx, userID ids.UserID, dealIDs []ids.UUID,
) (map[ids.UUID]CachedCard, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	found := map[ids.UUID]CachedCard{}
	where, args, err := cachedDealScope(ctx, userID, dealIDs)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT c.deal_id, c.payload, c.generated_at FROM deal_status_card c
		JOIN deal d ON d.id = c.deal_id WHERE `+where, args...)
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
		if !ok || card.access == nil || !cardSafeForDeal(card, dealID) || generatedAt.IsZero() {
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
		activities := append([]ids.UUID{}, card.CitedActivities...)
		if card.access != nil {
			activities = append(activities, card.access.Activities...)
		}
		for _, id := range activities {
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

func cardSafeForDeal(card CachedCard, dealID ids.UUID) bool {
	for _, citation := range card.Citations {
		switch citation.EntityType {
		case crmcontracts.CompanyBriefEvidenceEntityTypeActivity:
		case crmcontracts.CompanyBriefEvidenceEntityTypeDeal:
			if ids.UUID(citation.EntityId) != dealID {
				return false
			}
		default:
			return false
		}
	}
	return true
}
