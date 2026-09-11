// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Applying what the two matching tiers decided (ADR-0078 §2.1b, DECISIONS
// A143): a confirmation performs the whole write or it is not a confirmation. An
// automatic confirm is the same effect a human's approval releases — the
// connection linked, its profile URL stamped on the contact, and the audit rows
// and events the write shape owes for both — with a string comparison in the
// place of the human, so both land on confirmMatchWriteTail and cannot drift
// into two different meanings of "confirmed".
//
// The contact is HELD before the connection is touched, because an Art. 17
// erasure clears person_social and then deletes the connection, and a confirm
// that locked them the other way round would close a deadlock cycle against it.
// matchInTx locks every contact a confirm will touch — across BOTH tiers — in
// one ascending order before either tier is applied, so two concurrent passes
// acquire the same contacts in the same order and cannot cross. holdConfirmContacts
// is that lock; confirmOneMatch below relies on the contact already being held.

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// matchCandidate is one (ghost, contact) pair a tier decided to act on, and
// whether an exact identity released it for automatic confirmation. owner is the
// member whose imported network the ghost belongs to — it names the self-only
// decided event and binds the connection the confirm writes.
type matchCandidate struct {
	ghost       ids.UUID
	person      ids.UUID
	owner       ids.UUID
	confirmable bool
}

// scanCandidates reads the (ghost, person, owner, confirmable) rows both tiers
// return in that one shape, so a single reader serves both rather than two
// spellings of the same scan drifting apart.
func scanCandidates(rows pgx.Rows) ([]matchCandidate, error) {
	defer rows.Close()
	var out []matchCandidate
	for rows.Next() {
		var c matchCandidate
		if err := rows.Scan(&c.ghost, &c.person, &c.owner, &c.confirmable); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// confirmableContacts lists the distinct contacts a caller with the update grant
// would auto-confirm from these candidates — used to hold those contacts before
// applying, and to withhold an address-confirmed contact from the name tier.
func confirmableContacts(cands []matchCandidate) []ids.UUID {
	seen := map[ids.UUID]bool{}
	var out []ids.UUID
	for _, c := range cands {
		if c.confirmable && !seen[c.person] {
			seen[c.person] = true
			out = append(out, c.person)
		}
	}
	return out
}

// holdConfirmContacts locks every contact either tier will confirm, once and in
// one ascending order. That order is the whole point: person-before-connection
// keeps a confirm off the Art. 17 eraser's cycle, and a single ascending order
// across both tiers keeps two concurrent passes off each other's. A read-only
// pass confirms nothing and so is never called here.
func holdConfirmContacts(ctx context.Context, tx pgx.Tx, sets ...[]matchCandidate) error {
	seen := map[ids.UUID]bool{}
	var contacts []ids.UUID
	for _, set := range sets {
		for _, id := range confirmableContacts(set) {
			if !seen[id] {
				seen[id] = true
				contacts = append(contacts, id)
			}
		}
	}
	sort.Slice(contacts, func(i, j int) bool { return contacts[i].String() < contacts[j].String() })
	for _, id := range contacts {
		if err := auth.HoldWritableLive(ctx, tx, entityPerson, id); err != nil {
			return err
		}
	}
	return nil
}

// applyMatchCandidates writes each candidate's decision and reports how many
// confirmed and how many were left as suggestions.
//
// A confirmable candidate becomes a confirmation with the full write shape when
// the caller may edit a contact (canConfirm), and a suggestion when it may not:
// a read-only sweep produces suggestions and never edits a contact. Every
// non-confirmable candidate is a suggestion regardless. The counts are exact —
// one row per outcome — which a single RowsAffected over a mixed UPDATE could
// never report.
//
// Each confirmed contact must already be held by holdConfirmContacts; this is
// the write that assumes it.
func applyMatchCandidates(ctx context.Context, tx pgx.Tx, cands []matchCandidate, canConfirm bool) (confirmed, suggested int, err error) {
	for _, c := range cands {
		if c.confirmable && canConfirm {
			done, err := confirmOneMatch(ctx, tx, c)
			if err != nil {
				return 0, 0, err
			}
			if done {
				confirmed++
			}
			continue
		}
		done, err := suggestOneMatch(ctx, tx, c)
		if err != nil {
			return 0, 0, err
		}
		if done {
			suggested++
		}
	}
	return confirmed, suggested, nil
}

// confirmOneMatch links the ghost to its contact and performs the whole write.
// It reports false when the ghost is no longer there to confirm — a human
// decided it, or the address tier already claimed it earlier in this same pass —
// because confirming a row that moved is a link that does not exist.
func confirmOneMatch(ctx context.Context, tx pgx.Tx, c matchCandidate) (bool, error) {
	// The write-authority probe on the contact, and the contact held ahead of
	// the connection. holdConfirmContacts already took this lock in the pass's
	// one ascending order, so on the held row this is a no-op — but it keeps the
	// probe in the confirm's OWN path (a mutation of a contact must reach one)
	// and stands if this ever runs without the pre-lock.
	if err := auth.HoldWritableLive(ctx, tx, entityPerson, c.person); err != nil {
		return false, err
	}
	// The prior values come from the write itself, through a pre-write
	// self-join, so the audit attests to exactly the row this statement
	// replaced. match_status = 'unmatched' pins the automatic case: a ghost a
	// human or the address tier already moved is no longer ours to confirm.
	var wasStatus string
	var wasPerson *ids.UUID
	err := tx.QueryRow(ctx, `
		UPDATE linkedin_connection c
		   SET matched_person_id = $2, match_status = 'confirmed', updated_at = now()
		  FROM linkedin_connection was
		 WHERE c.id = $1 AND was.id = c.id
		   AND c.match_status = 'unmatched'
		   AND c.tombstoned_at IS NULL
		 RETURNING was.match_status, was.matched_person_id`,
		c.ghost, c.person).Scan(&wasStatus, &wasPerson)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("people: auto-confirming a LinkedIn match: %w", err)
	}
	if err := confirmMatchWriteTail(ctx, tx, c.ghost, c.owner, c.person, wasStatus, wasPerson); err != nil {
		return false, err
	}
	return true, nil
}

// suggestOneMatch records the pair as a proposal a human still owes a look. It
// writes only the ghost — the member's own row — so it needs no contact edit,
// no lock on the person, and no audit on the contact, because a suggestion
// changes nothing about the contact. It reports false when the ghost was decided
// out from under it.
func suggestOneMatch(ctx context.Context, tx pgx.Tx, c matchCandidate) (bool, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE linkedin_connection
		   SET matched_person_id = $2, match_status = 'suggested', updated_at = now()
		 WHERE id = $1 AND match_status = 'unmatched' AND tombstoned_at IS NULL`,
		c.ghost, c.person)
	if err != nil {
		return false, fmt.Errorf("people: suggesting a LinkedIn match: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
