// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// SupersededFields answers whether anyone wrote these keys after this audit
// row. It is deliberately NOT HumanOwnedConflicts (humanprecedence.go), which
// asks a different question: which keys a HUMAN last wrote to a DIFFERENT
// value, with no cutoff.
//
// The two must not share a reader. If this query inherited that one's
// equal-value exemption, a colleague who re-typed the same value would be
// invisible and the restore would silently revert a decision they had just
// re-affirmed. If that query inherited this one's actor-agnosticism or cutoff,
// fewer fields would read as human-owned and agent writes that stage for
// approval today would auto-execute — a silent weakening of an agent-authority
// guardrail.

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/privacy"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// auditCutoff is the point in the trail a supersession question is asked from:
// the target row's own position. audit_log is ordered by (occurred_at, id) —
// the id is a UUIDv7, so it breaks a tie in write order rather than arbitrarily
// — and the row itself is never "later" than itself.
type auditCutoff struct {
	OccurredAt time.Time
	ID         ids.UUID
}

// moneyPair is read as ONE field for supersession. amount_minor is a count of
// units the currency defines, so a later change to either makes a restore of
// the other state a value that never existed, wrong by the scale difference
// values.MinorUnitExceptions() encodes — and wrong silently, because the number
// is plausible in both denominations.
var moneyPair = []string{"amount_minor", "currency"}

// coupledKeys expands the keys a supersession question must actually ask about.
// A restore is refused per audit ROW, so pulling the sibling in here — rather
// than at each caller — is what keeps the coupling one rule instead of one per
// reader.
func coupledKeys(keys []string) []string {
	asked := make(map[string]bool, len(keys)+1)
	for _, key := range keys {
		asked[key] = true
	}
	for _, half := range moneyPair {
		if !asked[half] {
			continue
		}
		for _, other := range moneyPair {
			asked[other] = true
		}
		break
	}
	out := make([]string, 0, len(asked))
	for key := range asked {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// reportedAs maps a superseded key back onto the keys the caller asked about.
// The money pair travels together in both directions: a later change to the
// currency supersedes a restore of the amount, and the caller hears it named as
// the field it asked to put back.
func reportedAs(superseded []string, asked []string) []string {
	wasAsked := make(map[string]bool, len(asked))
	for _, key := range asked {
		wasAsked[key] = true
	}
	hit := make(map[string]bool, len(superseded))
	moneyMoved := false
	for _, key := range superseded {
		if wasAsked[key] {
			hit[key] = true
		}
		for _, half := range moneyPair {
			if key == half {
				moneyMoved = true
			}
		}
	}
	if moneyMoved {
		for _, half := range moneyPair {
			if wasAsked[half] {
				hit[half] = true
			}
		}
	}
	out := make([]string, 0, len(hit))
	for key := range hit {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// supersededFieldsTx is the query, taking the caller's transaction so the
// binding evaluation inside a write can ask it under the same reading. A second
// implementation reading its own snapshot is exactly the read-then-write shape
// that lets a stale answer decide a write's meaning.
//
// A row's OWN reversal chain is excluded. Putting an entry back writes the very
// fields the entry changed, and that write is later than the entry — so without
// this every restored entry would read as superseded by the act of restoring
// it. `already_undone` would be unreachable, and undoing an undo could never
// reopen anything, because the reopened entry would be permanently superseded
// by the two reversals that had cancelled each other out.
//
// The chain is followed transitively, not one link: a reversal of a reversal is
// as much a part of it as the first, and stopping at depth one would leave the
// same defect one press further along.
//
// Only `restore` rows join it. The evidence key is written by the reversal seam
// alone today, so no other write could enter the chain — but a chain that
// admitted any row carrying the key would let a future writer with caller-
// influenced evidence exclude its own write from superseding the entry it
// overwrote, and that is a guardrail nobody would notice losing.
func supersededFieldsTx(ctx context.Context, tx pgx.Tx, entityType string, id ids.UUID, keys []string, cutoff auditCutoff) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	asked := coupledKeys(keys)
	args := []any{entityType, id, asked, cutoff.OccurredAt, cutoff.ID, privacy.UndidAuditLogID}
	rows, err := tx.Query(ctx, `
		WITH RECURSIVE reversal_chain AS (
		  SELECT $5::uuid AS id
		  UNION
		  SELECT link.id
		  FROM audit_log link
		  JOIN reversal_chain c ON link.evidence ->> $6 = c.id::text
		  WHERE link.entity_type = $1 AND link.entity_id = $2
		    AND link.action = 'restore'
		)
		SELECT DISTINCT k.key
		FROM audit_log a
		CROSS JOIN unnest($3::text[]) AS k(key)
		WHERE a.entity_type = $1 AND a.entity_id = $2
		  AND a.after ? k.key
		  AND (a.occurred_at, a.id) > ($4, $5)
		  AND a.id NOT IN (SELECT id FROM reversal_chain)
		ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		found = append(found, key)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reportedAs(found, keys), nil
}
