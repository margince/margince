// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// How far apart the engine and the old purpose gate are, read from the record
// rather than guessed.
//
// The engine has run in observe mode since it shipped: it decides, records, and
// the old gate rules. That was always meant to end in a measurement — enforcing
// a rule nobody has measured is how a compliance change becomes an outage, and
// the one number that decides whether enforcement is safe is how often the two
// already disagree, broken down by what the disagreement WAS.
//
// A count alone would not answer it. "The engine is stricter on 4,000
// deliveries" reads as alarming and may be entirely correct — those may all be
// the legacy transactional purpose, which was an unconditional allow and is
// exactly what the engine exists to stop. What an operator needs is the shape:
// which category, which reason, and which direction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// disagreementLimit bounds the report. It groups rather than lists, so the
// cardinality is (categories × reasons × directions) and small by construction;
// the limit is a guard against a vocabulary that grew, not a page size.
const disagreementLimit = 200

// Disagreement is one shape of answer the two authorities gave differently, and
// how often.
type Disagreement struct {
	// Category is what the engine resolved the message to be.
	Category string
	// ReasonCode is why the engine answered as it did.
	ReasonCode string
	// EngineVerdict and LegacyVerdict are the two answers.
	EngineVerdict string
	LegacyVerdict string
	// Deliveries is how many distinct messages carry this shape. Distinct,
	// because one message to five recipients is one send an operator would have
	// to explain, not five.
	Deliveries int
	// Decisions is how many recipient-level decisions carry it.
	Decisions int
	// EngineIsStricter reports the direction that matters: the engine refusing
	// something the old gate allowed is what would stop mail on a flip. The
	// other direction is the engine permitting something the old gate refused,
	// which enforcement would START allowing.
	EngineIsStricter bool
}

// DisagreementReport reads how the engine and the old gate have differed.
//
// Gated on reading the installation's own settings rather than on a person:
// this discloses nothing about any subject — no address, no name, no consent
// state — only how two rules have compared across the installation. It is an
// operational question about the product, and the audience is whoever decides
// whether to enforce.
func (s *Store) DisagreementReport(ctx context.Context) ([]Disagreement, error) {
	if err := auth.Require(ctx, authorizationModesObject, principal.ActionRead); err != nil {
		return nil, err
	}
	var out []Disagreement
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT resolved_category, reason_code, verdict, legacy_verdict,
			       count(DISTINCT delivery_id), count(*)
			  FROM communication_decision
			 WHERE legacy_verdict IS NOT NULL
			   AND legacy_verdict <> verdict
			 GROUP BY resolved_category, reason_code, verdict, legacy_verdict
			 ORDER BY count(DISTINCT delivery_id) DESC, resolved_category, reason_code
			 LIMIT $1`, disagreementLimit)
		if err != nil {
			return fmt.Errorf("consent: read how the two authorities have differed: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var d Disagreement
			if err := rows.Scan(&d.Category, &d.ReasonCode, &d.EngineVerdict,
				&d.LegacyVerdict, &d.Deliveries, &d.Decisions); err != nil {
				return fmt.Errorf("consent: read how the two authorities have differed: %w", err)
			}
			// The old gate answers allow or deny; the engine also answers
			// review. Anything that is not an allow would stop the send once
			// the engine rules, so review counts as stricter here — an operator
			// asking "what would enforcing cost me" is asking about mail that
			// stops, and a review stops it just as a deny does.
			d.EngineIsStricter = d.LegacyVerdict == string(commsauthz.VerdictAllow) &&
				d.EngineVerdict != string(commsauthz.VerdictAllow)
			out = append(out, d)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
