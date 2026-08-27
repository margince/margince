// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NotPromotedError maps to 409: the lead was never promoted, or its
// promotion has already been reversed — either way there is nothing to undo.
type NotPromotedError struct{}

func (e *NotPromotedError) Error() string { return "lead is not promoted; nothing to reverse" }

// PersonHasDealError maps to 422 person_has_deal (formulas §26.1): the
// promoted person carries commercial state, and un-personing it would strand
// the deal's counterparty. The person stays a person.
type PersonHasDealError struct{}

func (e *PersonHasDealError) Error() string {
	return "the promoted person is a stakeholder on a live deal; the promotion cannot be reversed"
}

// MessageFault names the condition and no field: the remedy is on the deal,
// not on any input of this request.
func (e *PersonHasDealError) MessageFault() (code, message string) {
	return "person_has_deal", e.Error()
}

// fieldKeyPromotedPerson is the lead's outcome pointer as it appears in
// audit images and problem details.
const fieldKeyPromotedPerson = "promoted_person_id"

// promotionOutcome is what the promote audit row recorded as
// dedupe_outcome; the unwind is decided from it, never re-derived.
type promotionOutcome string

const (
	outcomeCreated promotionOutcome = "created"
	outcomeMerged  promotionOutcome = "merged"
)

// DemoteLead reverses a promotion (formulas §26, the undo ADR-0008 §4
// promises). It is deterministic and conservative — it blocks rather than
// orphans: a person who is a stakeholder on a live deal cannot revert. A
// promotion that CREATED the person archives that person and restores the
// lead; one that MERGED into a pre-existing person leaves the person alone
// and only nulls the lineage. Activities stay where they are — captured
// history is not rewritten backwards. One audit row, lead.demoted, all in one
// transaction under the lead row lock.
func (s *Store) DemoteLead(ctx context.Context, id ids.LeadID, reason string) (crmcontracts.DemoteLeadResponse, error) {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return crmcontracts.DemoteLeadResponse{}, err
	}
	active, err := s.activeColumns(ctx, "lead")
	if err != nil {
		return crmcontracts.DemoteLeadResponse{}, err
	}
	var out crmcontracts.DemoteLeadResponse
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// PERSON BEFORE LEAD, which is the order MergePerson takes: it locks
		// the two people (LockPair) and then repoints lead.promoted_person_id,
		// and an UPDATE locks the row it writes. Two writers taking the same
		// pair in opposite orders is the whole of a deadlock — each holds what
		// the other waits for, Postgres aborts one, and the caller gets a 5xx
		// where the losing side of a serialized race should get a clean
		// refusal.
		//
		// Which person to lock is written on the lead, so the lead is read
		// FIRST and unlocked. That read is a hint, not a decision: the lead is
		// re-read under both locks below and the answer is taken from there.
		personID, err := promotedPersonOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "person", personID.UUID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("lock promoted person: %w", err)
		}
		// The lead lock serializes a demote against a concurrent re-promote or
		// second demote; the loser re-reads the state and answers 409.
		if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
		lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
		if err != nil {
			return fmt.Errorf("read lead before demote: %w", err)
		}
		// Re-checked UNDER the locks, against the person actually locked. The
		// unlocked read above can be overtaken — by a merge repointing this
		// lead at the survivor, or by a demote that got there first — and
		// proceeding on it would unwind a person this lead no longer names.
		if lead.Status != crmcontracts.LeadStatusPromoted || lead.PromotedPersonId == nil {
			return &NotPromotedError{}
		}
		if ids.UUID(*lead.PromotedPersonId) != personID.UUID {
			// Somebody moved this lead's person between the two reads. Refused
			// rather than retried here: the caller re-issues against a lead
			// whose state they can see, which is the same answer a second
			// demote gets.
			return &NotPromotedError{}
		}

		outcome, err := promotedOutcome(ctx, tx, id)
		if err != nil {
			return err
		}
		unwind, err := unwindPerson(ctx, tx, id, personID, outcome)
		if err != nil {
			return err
		}
		setBy, err := statusSetByFor(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE lead SET status = 'engaged', status_set_by = $2, archived_at = NULL,
			        promoted_person_id = NULL, promoted_at = NULL, qualified_deal_id = NULL
			 WHERE id = $1`, id, setBy); err != nil {
			return fmt.Errorf("restore lead: %w", err)
		}

		auditID, err := storekit.Audit(ctx, tx, "demote", "lead", id.UUID,
			map[string]any{leadStatusColumn: lead.Status, fieldKeyPromotedPerson: personID},
			map[string]any{leadStatusColumn: string(LeadStatusEngaged), "unwind": unwind, fieldKeyReason: reason})
		if err != nil {
			return fmt.Errorf("audit lead demote: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadDemoted{
			FromPersonId: openapi_types.UUID(personID.UUID), Unwind: string(unwind),
		}); err != nil {
			return fmt.Errorf("emit lead.demoted: %w", err)
		}

		restored, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read restored lead: %w", err)
		}
		pid := openapi_types.UUID(personID.UUID)
		out = crmcontracts.DemoteLeadResponse{Lead: restored, Unwind: unwind, PersonId: &pid}
		return nil
	})
	return out, err
}

// promotedOutcome reads what the promotion actually did from its audit row.
// Re-running the dedupe ladder would answer about today's data, not about
// the promotion being reversed. An unreadable outcome refuses rather than
// guesses: archiving a person the promotion did not create is the one
// mistake this verb must never make.
func promotedOutcome(ctx context.Context, tx pgx.Tx, id ids.LeadID) (promotionOutcome, error) {
	var recorded *string
	err := tx.QueryRow(ctx, `
		SELECT after->>'dedupe_outcome' FROM audit_log
		WHERE entity_type = 'lead' AND entity_id = $1 AND action = 'promote'
		ORDER BY occurred_at DESC LIMIT 1`, id).Scan(&recorded)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && recorded == nil) {
		return "", fmt.Errorf("promotion audit row carries no dedupe_outcome: %w", apperrors.ErrConflict)
	}
	if err != nil {
		return "", fmt.Errorf("read promotion outcome: %w", err)
	}
	switch o := promotionOutcome(*recorded); o {
	case outcomeCreated, outcomeMerged:
		return o, nil
	}
	return "", fmt.Errorf("promotion audit row carries an unknown dedupe_outcome: %w", apperrors.ErrConflict)
}

// ensureNoLiveDeal is the §26.1 hard block: a person attached to a live deal
// as a stakeholder keeps commercial state that a demotion would strand.
func ensureNoLiveDeal(ctx context.Context, tx pgx.Tx, personID ids.PersonID) error {
	var hasDeal bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM relationship r
		  JOIN deal d ON d.id = r.deal_id AND d.archived_at IS NULL
		  WHERE r.person_id = $1 AND r.kind = 'deal_stakeholder' AND r.archived_at IS NULL)`,
		personID).Scan(&hasDeal); err != nil {
		return fmt.Errorf("check person deals: %w", err)
	}
	if hasDeal {
		return &PersonHasDealError{}
	}
	return nil
}

// unwindPerson applies the person side of §26.1 and names what it did.
// Both unwinds write the person (the lineage pointer at least), so the
// person's grant and row scope are checked and the row locked BEFORE anything
// is read about it: the deal probe must not tell a caller who cannot see the
// person whether it sits on a live deal. Archiving the created person needs
// no separate archive grant: the promotion that minted it ran under
// lead.update + person.create, and its reversal is that same authority
// exercised backwards — a rep who may promote must be able to undo the
// promotion (ADR-0008 §4), and the default rep role holds no person.delete.
//
// The archive branch has two more guards than the audit outcome: the person
// must still be ONLY what the promotion minted, and nothing else may depend
// on it. A person merge repoints lead.promoted_person_id to its survivor, and
// a survivor holds other people's history; a later lead promoted INTO this
// person points its own contact surface at it. Archiving in either case would
// destroy or strand records the promotion never created, so both unwind
// lineage-only, whatever the promotion originally did.
func unwindPerson(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, personID ids.PersonID, outcome promotionOutcome) (crmcontracts.DemoteLeadResponseUnwind, error) {
	if err := auth.Require(ctx, "person", principal.ActionUpdate); err != nil {
		return "", err
	}
	if err := auth.EnsureWritable(ctx, tx, "person", personID.UUID); err != nil {
		return "", err
	}
	// The caller already holds this lock: it takes it BEFORE the lead, which is
	// the order MergePerson takes and the reason it cannot be acquired here for
	// the first time. Re-taking it is nearly free and is kept so this function's
	// own by-id UPDATE below is guarded by something in this function, rather
	// than by a caller a reader has to go and find.
	if _, err := storekit.LockRow(ctx, tx, "person", personID.UUID, storekit.LiveOnly); err != nil {
		return "", fmt.Errorf("lock promoted person: %w", err)
	}
	if err := ensureNoLiveDeal(ctx, tx, personID); err != nil {
		return "", err
	}
	if outcome == outcomeCreated {
		shared, err := isSharedByOthers(ctx, tx, leadID, personID)
		if err != nil {
			return "", err
		}
		if shared {
			outcome = outcomeMerged
		}
	}
	if outcome == outcomeMerged {
		if _, err := tx.Exec(ctx,
			`UPDATE person SET converted_from_lead_id = NULL WHERE id = $1 AND converted_from_lead_id = $2`,
			personID, leadID); err != nil {
			return "", fmt.Errorf("null merge lineage: %w", err)
		}
		return crmcontracts.DemoteUnwindMergeLineageOnly, nil
	}
	if err := archivePersonRows(ctx, tx, personID, time.Now().UTC(), nil); err != nil {
		return "", fmt.Errorf("archive promoted person: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE person SET converted_from_lead_id = NULL WHERE id = $1`, personID); err != nil {
		return "", fmt.Errorf("null created lineage: %w", err)
	}
	return crmcontracts.DemoteUnwindReversed, nil
}

// isSharedByOthers answers whether records beyond this promotion depend on
// the person: another person row merged into it, or another lead promoted
// into it.
func isSharedByOthers(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, personID ids.PersonID) (bool, error) {
	var shared bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM person WHERE merged_into_id = $1)
		    OR EXISTS (SELECT 1 FROM lead WHERE promoted_person_id = $1 AND id <> $2)`,
		personID, leadID).Scan(&shared); err != nil {
		return false, fmt.Errorf("check dependants of the promoted person: %w", err)
	}
	return shared, nil
}

// promotedPersonOf reads which person a lead was promoted into, WITHOUT taking
// a lock.
//
// The demote has to lock that person before it locks the lead — the order the
// merge path takes — and the person's id is written on the lead, so something
// has to read it first. This read is a hint: the caller locks what it names,
// then re-reads the lead under both locks and refuses if the two disagree.
func promotedPersonOf(ctx context.Context, tx pgx.Tx, id ids.LeadID) (ids.PersonID, error) {
	var promoted *ids.UUID
	if err := tx.QueryRow(ctx,
		`SELECT promoted_person_id FROM lead WHERE id = $1`, id.UUID).Scan(&promoted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ids.PersonID{}, apperrors.ErrNotFound
		}
		return ids.PersonID{}, fmt.Errorf("read the lead's promoted person: %w", err)
	}
	if promoted == nil {
		return ids.PersonID{}, &NotPromotedError{}
	}
	return ids.From[ids.PersonKind](*promoted), nil
}
