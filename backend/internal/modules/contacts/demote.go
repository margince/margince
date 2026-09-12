// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

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

// ContactHasDealError maps to 422 contact_has_deal (formulas §26.1): the
// promoted contact carries commercial state, and un-contacting it would strand
// the deal's counterparty. The contact stays a contact.
type ContactHasDealError struct{}

func (e *ContactHasDealError) Error() string {
	return "the promoted contact is a stakeholder on a live deal; the promotion cannot be reversed"
}

// MessageFault names the condition and no field: the remedy is on the deal,
// not on any input of this request.
func (e *ContactHasDealError) MessageFault() (code, message string) {
	return "contact_has_deal", e.Error()
}

// fieldKeyPromotedContact is the lead's outcome pointer as it appears in
// audit images and problem details.
const fieldKeyPromotedContact = "promoted_contact_id"

// promotionOutcome is what the promote audit row recorded as
// dedupe_outcome; the unwind is decided from it, never re-derived.
type promotionOutcome string

const (
	outcomeCreated promotionOutcome = "created"
	outcomeMerged  promotionOutcome = "merged"
)

// DemoteLead reverses a promotion (formulas §26, the undo ADR-0008 §4
// promises). It is deterministic and conservative — it blocks rather than
// orphans: a contact who is a stakeholder on a live deal cannot revert. A
// promotion that CREATED the contact archives that contact and restores the
// lead; one that MERGED into a pre-existing contact leaves the contact alone
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
		// CONTACT BEFORE LEAD, which is the order MergeContact takes: it locks
		// the two contacts (LockPair) and then repoints lead.promoted_contact_id,
		// and an UPDATE locks the row it writes. Two writers taking the same
		// pair in opposite orders is the whole of a deadlock — each holds what
		// the other waits for, Postgres aborts one, and the caller gets a 5xx
		// where the losing side of a serialized race should get a clean
		// refusal.
		//
		// Which contact to lock is written on the lead, so the lead is read
		// FIRST and unlocked. That read is a hint, not a decision: the lead is
		// re-read under both locks below and the answer is taken from there.
		//
		// BOTH row-scope probes therefore run before either lock, and the lead's
		// runs before anything is read off it at all. Under lead-then-contact the
		// scope check sat behind the lead lock and ahead of every read, so
		// nothing about the row could be learned by a caller it would refuse.
		// Reading the lead first to name the human moves that read in front of
		// the check, and unprobed it would answer a caller who may not see this
		// lead whether it was ever promoted — "not promoted" rather than "not
		// yours" — and then take a lock on a contact they cannot see. This is
		// unwindContact's own rule, which its doc states for the deal probe, owed
		// one frame earlier because the lock moved one frame earlier.
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return err
		}
		contactID, err := promotedContactOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
			return err
		}
		if err := auth.EnsureWritable(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, "contact", contactID.UUID, storekit.LiveOnly); err != nil {
			return fmt.Errorf("lock promoted contact: %w", err)
		}
		// The lead lock serializes a demote against a concurrent re-promote or
		// second demote; the loser re-reads the state and answers 409.
		if _, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
		lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
		if err != nil {
			return fmt.Errorf("read lead before demote: %w", err)
		}
		// Re-checked UNDER the locks, against the contact actually locked. The
		// unlocked read above can be overtaken — by a merge repointing this
		// lead at the survivor, or by a demote that got there first — and
		// proceeding on it would unwind a contact this lead no longer names.
		if lead.Status != crmcontracts.LeadStatusPromoted || lead.PromotedContactId == nil {
			return &NotPromotedError{}
		}
		if ids.UUID(*lead.PromotedContactId) != contactID.UUID {
			// Somebody moved this lead's contact between the two reads. Refused
			// rather than retried here: the caller re-issues against a lead
			// whose state they can see, which is the same answer a second
			// demote gets.
			return &NotPromotedError{}
		}

		outcome, err := promotedOutcome(ctx, tx, id)
		if err != nil {
			return err
		}
		unwind, err := unwindContact(ctx, tx, id, contactID, outcome)
		if err != nil {
			return err
		}
		setBy, err := statusSetByFor(ctx)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE lead SET status = 'engaged', status_set_by = $2, archived_at = NULL,
			        promoted_contact_id = NULL, promoted_at = NULL, qualified_deal_id = NULL
			 WHERE id = $1`, id, setBy); err != nil {
			return fmt.Errorf("restore lead: %w", err)
		}

		auditID, err := storekit.Audit(ctx, tx, "demote", "lead", id.UUID,
			map[string]any{leadStatusColumn: lead.Status, fieldKeyPromotedContact: contactID},
			map[string]any{leadStatusColumn: string(LeadStatusEngaged), "unwind": unwind, fieldKeyReason: reason})
		if err != nil {
			return fmt.Errorf("audit lead demote: %w", err)
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventLeadDemoted{
			FromContactId: openapi_types.UUID(contactID.UUID), Unwind: string(unwind),
		}); err != nil {
			return fmt.Errorf("emit lead.demoted: %w", err)
		}

		restored, err := readLead(ctx, tx, id, storekit.LiveOnly, active)
		if err != nil {
			return fmt.Errorf("read restored lead: %w", err)
		}
		pid := openapi_types.UUID(contactID.UUID)
		out = crmcontracts.DemoteLeadResponse{Lead: restored, Unwind: unwind, ContactId: &pid}
		return nil
	})
	return out, err
}

// promotedOutcome reads what the promotion actually did from its audit row.
// Re-running the dedupe ladder would answer about today's data, not about
// the promotion being reversed. An unreadable outcome refuses rather than
// guesses: archiving a contact the promotion did not create is the one
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

// ensureNoLiveDeal is the §26.1 hard block: a contact attached to a live deal
// as a stakeholder keeps commercial state that a demotion would strand.
func ensureNoLiveDeal(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) error {
	var hasDeal bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM relationship r
		  JOIN deal d ON d.id = r.deal_id AND d.archived_at IS NULL
		  WHERE r.contact_id = $1 AND r.kind = 'deal_stakeholder' AND r.archived_at IS NULL)`,
		contactID).Scan(&hasDeal); err != nil {
		return fmt.Errorf("check contact deals: %w", err)
	}
	if hasDeal {
		return &ContactHasDealError{}
	}
	return nil
}

// unwindContact applies the contact side of §26.1 and names what it did.
// Both unwinds write the contact (the lineage pointer at least), so the
// contact's grant and row scope are checked and the row locked BEFORE anything
// is read about it: the deal probe must not tell a caller who cannot see the
// contact whether it sits on a live deal. Its one caller now owes the same
// three one frame earlier — it locks the contact before the lead — so these
// re-ask a question already answered. They are kept rather than deleted
// because they are what makes THIS function safe to read and to call: a guard
// that lives only in a caller is a guard the next caller does not get.
// Archiving the created contact needs
// no separate archive grant: the promotion that minted it ran under
// lead.update + contact.create, and its reversal is that same authority
// exercised backwards — a rep who may promote must be able to undo the
// promotion (ADR-0008 §4), and the default rep role holds no contact.delete.
//
// The archive branch has two more guards than the audit outcome: the contact
// must still be ONLY what the promotion minted, and nothing else may depend
// on it. A contact merge repoints lead.promoted_contact_id to its survivor, and
// a survivor holds other contacts's history; a later lead promoted INTO this
// contact points its own contact surface at it. Archiving in either case would
// destroy or strand records the promotion never created, so both unwind
// lineage-only, whatever the promotion originally did.
func unwindContact(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID, outcome promotionOutcome) (crmcontracts.DemoteLeadResponseUnwind, error) {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return "", err
	}
	if err := auth.EnsureWritable(ctx, tx, "contact", contactID.UUID); err != nil {
		return "", err
	}
	// The caller already holds this lock: it takes it BEFORE the lead, which is
	// the order MergeContact takes and the reason it cannot be acquired here for
	// the first time. Re-taking it is nearly free and is kept so this function's
	// own by-id UPDATE below is guarded by something in this function, rather
	// than by a caller a reader has to go and find.
	if _, err := storekit.LockRow(ctx, tx, "contact", contactID.UUID, storekit.LiveOnly); err != nil {
		return "", fmt.Errorf("lock promoted contact: %w", err)
	}
	if err := ensureNoLiveDeal(ctx, tx, contactID); err != nil {
		return "", err
	}
	if outcome == outcomeCreated {
		shared, err := isSharedByOthers(ctx, tx, leadID, contactID)
		if err != nil {
			return "", err
		}
		if shared {
			outcome = outcomeMerged
		}
	}
	if outcome == outcomeMerged {
		if _, err := tx.Exec(ctx,
			`UPDATE contact SET converted_from_lead_id = NULL WHERE id = $1 AND converted_from_lead_id = $2`,
			contactID, leadID); err != nil {
			return "", fmt.Errorf("null merge lineage: %w", err)
		}
		return crmcontracts.DemoteLeadResponseUnwindDemoteUnwindMergeLineageOnly, nil
	}
	if err := archiveContactRows(ctx, tx, contactID, time.Now().UTC(), nil); err != nil {
		return "", fmt.Errorf("archive promoted contact: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE contact SET converted_from_lead_id = NULL WHERE id = $1`, contactID); err != nil {
		return "", fmt.Errorf("null created lineage: %w", err)
	}
	return crmcontracts.DemoteLeadResponseUnwindDemoteUnwindReversed, nil
}

// isSharedByOthers answers whether records beyond this promotion depend on
// the contact: another contact row merged into it, or another lead promoted
// into it.
func isSharedByOthers(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID) (bool, error) {
	var shared bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM contact WHERE merged_into_id = $1)
		    OR EXISTS (SELECT 1 FROM lead WHERE promoted_contact_id = $1 AND id <> $2)`,
		contactID, leadID).Scan(&shared); err != nil {
		return false, fmt.Errorf("check dependants of the promoted contact: %w", err)
	}
	return shared, nil
}

// promotedContactOf reads which contact a lead was promoted into, WITHOUT taking
// a lock.
//
// The demote has to lock that contact before it locks the lead — the order the
// merge path takes — and the contact's id is written on the lead, so something
// has to read it first. This read is a hint: the caller locks what it names,
// then re-reads the lead under both locks and refuses if the two disagree.
func promotedContactOf(ctx context.Context, tx pgx.Tx, id ids.LeadID) (ids.ContactID, error) {
	var promoted *ids.UUID
	if err := tx.QueryRow(ctx,
		`SELECT promoted_contact_id FROM lead WHERE id = $1`, id.UUID).Scan(&promoted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ids.ContactID{}, apperrors.ErrNotFound
		}
		return ids.ContactID{}, fmt.Errorf("read the lead's promoted contact: %w", err)
	}
	if promoted == nil {
		return ids.ContactID{}, &NotPromotedError{}
	}
	return ids.From[ids.ContactKind](*promoted), nil
}
