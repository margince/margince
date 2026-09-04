// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Contacts the capture made and never asked about.
//
// A person minted from a message nobody has judged is the mailbox owner's until
// a verdict says otherwise. The question that reaches that verdict is opened at
// capture, and it can be refused: the ceiling on open questions is per workspace
// and per domain, and a refusal writes nothing. The refusal is deliberate — the
// question is delayed, not cancelled — but the retry rides the NEXT message from
// that address, and a correspondence that has gone quiet never sends one.
//
// The contact then stays owner-private for good: invisible to every colleague,
// to their manager, and to an admin. Nothing re-asks, because the thing that
// would put it back in the queue is the row the ceiling refused to write.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StrandedContact is one such person and the message to ask about them with.
type StrandedContact struct {
	PersonID    ids.UUID
	OwnerID     ids.UUID
	Email       string
	Domain      string
	DisplayName string
	ActivityID  ids.UUID
}

// StrandedContacts lists the connector-made owner-private people no ledger row
// has ever been written for.
//
// Any status counts as asked, not merely a live one: a terminal `advisor` is an
// answer, and re-asking a settled question would put a decided sender back in
// front of a model. That is the same rule the capture path applies.
//
// The page is drawn at RANDOM rather than in a stable order. A refusal writes
// nothing, so a refused contact is offered again — and under a stable order the
// same 200 rows are offered every tick. Two hundred contacts at a domain whose
// own ceiling is full would then be retried daily forever while every other
// domain behind them is never reached, and the sweep would report success the
// whole time. Randomising costs a sort the bound already pays for and makes
// progress a matter of ticks rather than of luck with the ordering.
//
// Ownerless rows are excluded rather than repaired. `person.owner_id` is
// nullable while `visibility` is independently allowed to say `owner`, and
// nothing in the schema ties the two; a row with no owner cannot be asked about
// on anybody's behalf, and quietly picking one would assign somebody else's
// correspondence to a seat that never saw it.
//
// A contact a HUMAN has touched is left alone, on the same evidence
// people.RetractCaptureOnlyPersonTx uses: an audit row with a human actor. That
// path deliberately refuses to retract such a contact, so a person kept
// owner-private after somebody worked on it is a decision, and asking again
// would put it in front of a model that can promote it — a transition nothing
// reverses.
//
// One row per ADDRESS, and each is asked about with a message from that
// address. A person with several addresses can be a business contact at one and
// nobody's business at another; grounding both questions in whichever message
// happened to be newest would judge one address by the other's correspondence.
func (s *PendingStore) StrandedContacts(ctx context.Context, limit int) ([]StrandedContact, error) {
	var out []StrandedContact
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.id, p.owner_id, pe.email,
			       coalesce(nullif(split_part(pe.email, '@', 2), ''), ''),
			       coalesce(p.full_name, ''),
			       (SELECT a.id
			          FROM activity_link l
			          JOIN activity a ON a.id = l.activity_id
			         WHERE l.entity_type = 'person' AND l.person_id = p.id
			           AND a.captured_by LIKE 'connector:%'
			           AND lower(btrim(coalesce(a.counterparty_email, ''))) = pe.email
			           AND a.archived_at IS NULL AND a.restricted_at IS NULL
			         ORDER BY a.occurred_at DESC, a.id DESC
			         LIMIT 1)
			  FROM person p
			  JOIN person_email pe ON pe.person_id = p.id AND pe.archived_at IS NULL
			 WHERE p.visibility = 'owner'
			   AND p.archived_at IS NULL
			   AND p.merged_into_id IS NULL
			   AND p.owner_id IS NOT NULL
			   AND p.captured_by LIKE 'connector:%'
			   AND NOT EXISTS (SELECT 1 FROM capture_pending_counterparty q
			                    WHERE q.email = pe.email)
			   AND NOT EXISTS (SELECT 1 FROM audit_log al
			                    WHERE al.entity_type = 'person' AND al.entity_id = p.id
			                      AND al.actor_type = 'human')
			 ORDER BY random()
			 LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c StrandedContact
			var activity *ids.UUID
			if err := rows.Scan(&c.PersonID, &c.OwnerID, &c.Email,
				&c.Domain, &c.DisplayName, &activity); err != nil {
				return err
			}
			if activity == nil {
				// No captured message left to ask about — erased, archived, or
				// under a hold. The question needs one: the ledger row names
				// the activity a verdict was raised over.
				continue
			}
			c.ActivityID = *activity
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing captured contacts nobody was asked about: %w", err)
	}
	return out, nil
}

// NoiseJudgedContact is one capture-made person still standing at an address a
// settled verdict already called noise.
type NoiseJudgedContact struct {
	PersonID ids.PersonID
	OwnerID  ids.UUID
	Email    string
}

// NoiseJudgedContacts lists the connector-made, owner-private people an
// already-settled answer has disowned: their address settled as `newsletter`,
// `transactional` or `spam`, or their owner recorded a standing `keep_out`. A
// settled sender never re-enters the ledger, so no future verdict reaches
// these — this selector is what does.
//
// The bounds mirror the retraction they feed:
//
//   - An owner's answer (`keep out`, or an owner-resolved noise row) claims
//     only the DECIDER's own record; a colleague's copy of the address is
//     offered only for a machine verdict. That is the two owner clauses.
//   - Against a machine noise row, a live question or a `real` verdict
//     outranks it: the address's standing is contested or settled the other
//     way, and retracting on the stale answer would act on the losing one. An
//     owner's keep_out is not outranked — the engine itself consults the
//     override before any model and never writes over it.
//   - A contact a HUMAN has touched is excluded on the same evidence
//     people.RetractCaptureOnlyPersonTx refuses it on — an audit row with a
//     human actor. The retraction re-checks; excluding here keeps a page from
//     filling with rows the retraction will refuse every tick.
//
// The page is drawn at random, like StrandedContacts and for the same reason:
// a contact the retraction refuses (a corresponded sender, checked at retract
// time) stays in the selection, and under a stable order a page of refusals
// would starve every row behind it forever while reporting success.
func (s *PendingStore) NoiseJudgedContacts(ctx context.Context, limit int) ([]NoiseJudgedContact, error) {
	var out []NoiseJudgedContact
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT p.id, p.owner_id, pe.email
			  FROM person p
			  JOIN person_email pe ON pe.person_id = p.id AND pe.archived_at IS NULL
			 WHERE p.visibility = 'owner'
			   AND p.archived_at IS NULL
			   AND p.merged_into_id IS NULL
			   AND p.owner_id IS NOT NULL
			   AND p.captured_by LIKE 'connector:%'
			   AND `+noiseJudgedStandsSQL("pe.email", "p.owner_id")+`
			   AND NOT EXISTS (SELECT 1 FROM audit_log al
			                    WHERE al.entity_type = 'person' AND al.entity_id = p.id
			                      AND al.actor_type = 'human')
			 ORDER BY random()
			 LIMIT $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c NoiseJudgedContact
			if err := rows.Scan(&c.PersonID, &c.OwnerID, &c.Email); err != nil {
				return err
			}
			out = append(out, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("capture: listing the contacts a noise verdict already covered: %w", err)
	}
	return out, nil
}

// noiseJudgedStandsSQL is the ONE spelling of "an already-settled answer
// disowns this contact": a machine noise verdict no live question or `real`
// verdict outranks, or the owner's own standing keep_out. The selector above
// and the per-retraction recheck both build from it, so the scan and the write
// cannot drift into asking different questions. Both arguments are
// compile-time SQL expressions, never caller data.
func noiseJudgedStandsSQL(emailExpr, ownerExpr string) string {
	return `((EXISTS (SELECT 1 FROM capture_pending_counterparty q
	                   WHERE q.email = ` + emailExpr + `
	                     AND q.status = 'noise'
	                     AND q.kind IN ('newsletter', 'transactional', 'spam')
	                     AND (NOT q.resolved_by_owner OR q.owner_id = ` + ownerExpr + `))
	          AND NOT EXISTS (SELECT 1 FROM capture_pending_counterparty q2
	                           WHERE q2.email = ` + emailExpr + `
	                             AND q2.status IN ('pending', 'unsure', 'real')))
	         OR EXISTS (SELECT 1 FROM capture_sender_override o
	                     WHERE o.address = ` + emailExpr + `
	                       AND o.decision = 'keep_out'
	                       AND o.user_id = ` + ownerExpr + `))`
}

// NoiseJudgedStandsTx re-reads, on the retraction's own transaction, whether
// the answer that selected a contact still stands. The scan and the archive
// are separate transactions, so a keep_out withdrawn — or a verdict corrected
// — between them would otherwise still cost the contact.
func (s *PendingStore) NoiseJudgedStandsTx(ctx context.Context, tx pgx.Tx, email string, ownerID ids.UUID) (bool, error) {
	var stands bool
	if err := tx.QueryRow(ctx, `SELECT `+noiseJudgedStandsSQL("$1", "$2"),
		email, ownerID).Scan(&stands); err != nil {
		return false, fmt.Errorf("capture: re-reading whether a noise answer still stands: %w", err)
	}
	return stands, nil
}

// AskWhoseRecord opens the question the capture could not.
//
// The same write the capture path makes, through askWhoseRecordTx, which is
// where the ceiling, the terminal-answer check and the suppression check are
// applied to both callers alike. The ceiling still applies: a
// refusal writes nothing and reports so, and the contact is offered again next
// tick — bounded by the caller's own batch, and self-clearing as the queue
// drains.
func (s *PendingStore) AskWhoseRecord(ctx context.Context, c StrandedContact) (bool, error) {
	asked := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		wrote, err := askWhoseRecordTx(ctx, tx, dispositionRow{
			Email:       c.Email,
			Domain:      c.Domain,
			DisplayName: c.DisplayName,
			ActivityID:  c.ActivityID,
			OwnerID:     c.OwnerID,
		})
		if err != nil || !wrote {
			return err
		}
		asked = true
		// The trail the capture path gets from the activity write it rides
		// inside. This sweep has no such write of its own, and a question that
		// can end in a contact becoming visible to the workspace should not be
		// the one mutation with nothing recording that it was raised.
		//
		// On the PERSON, not the ledger row: "why is this contact suddenly in
		// front of everybody" is a question asked about the person, and the
		// answer has to be findable from them.
		if _, err := storekit.AuditEvent(ctx, tx, "update", "person", c.PersonID,
			map[string]any{"capture_question": "reopened", kindEmail: c.Email}); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("capture: asking whose record a captured contact is: %w", err)
	}
	return asked, nil
}
