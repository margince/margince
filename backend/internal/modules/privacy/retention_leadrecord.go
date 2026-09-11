// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// What retention does to the communication record of a LEAD.
//
// Separate from the contact arm in retentionactions.go because the two reach
// their rows by different columns — lead_id against contact_id — and separate
// from erasure_leadtwins.go because that path is an Art. 17 erasure: it deletes
// a suppression and keeps the refusal alive as an erasure_suppression hash,
// where this one has no hash to write and must carry the objection forward on
// the address instead.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (*RetentionService) anonymizeLead(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	// BEFORE the UPDATE below nulls it: a suppression is detached onto the
	// address it applies to, and the lead row is the only place that address
	// still lives. Read after the scrub and the objection is orphaned.
	// A lead that is already gone has no address to carry, which the UPDATE
	// below treats the same way: nothing matches and nothing is scrubbed.
	// FOR UPDATE: the address read here is detached onto the suppression below,
	// and the UPDATE that follows nulls it. Without the lock a concurrent write
	// could change the address between the two, detaching the objection onto an
	// address the lead no longer uses — so a re-capture from the new one comes
	// back mailable.
	var address *string
	if err := tx.QueryRow(ctx,
		`SELECT email FROM lead WHERE id = $1 FOR UPDATE`, id).Scan(&address); err != nil {
		return fmt.Errorf("read the lead's address before scrubbing it: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE lead SET full_name = 'Anonymized Lead', email = NULL, title = NULL,
		  company_name = NULL, candidate_company_key = NULL, raw = NULL, linkedin_url = NULL,
		  disqualify_note = NULL, score_override_reason = NULL, archived_at = coalesce(archived_at, now())
		WHERE id = $1`, id); err != nil {
		return err
	}
	// The score's explanation goes with the lead it explains. Both tables
	// hold personal data the UPDATE above cannot reach: the retained series
	// embeds activity ids inside its factors JSON, and a manual signal names
	// the colleague who entered it and carries their written reason. This is
	// an ANONYMIZE, not a delete, so the lead row survives and fires no
	// ON DELETE cascade — the FKs on those tables do nothing here, which is
	// why each has to be named (ADR-0105).
	if _, err := tx.Exec(ctx, `DELETE FROM lead_score_history WHERE lead_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM lead_manual_signal WHERE lead_id = $1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM embedding WHERE entity_type = 'lead' AND entity_id = $1`, id); err != nil {
		return err
	}
	// The lead's communication record, for the same reason as the two tables
	// above: an anonymize fires no cascade, so each table that names the lead
	// has to be named here. A basis row carries the thread it was earned on and
	// the message that earned it; a decision row carries the address the mail
	// went to. Both outlive an UPDATE that only scrubs the lead row.
	//
	// NOT the same as erasure_leadtwins.go's lead arm, which DELETES the
	// suppression: that path is an Art. 17 erasure and writes an
	// erasure_suppression hash to keep the refusal alive. This one writes none,
	// so the suppression is detached onto the address instead.
	return clearLeadCommunicationRecord(ctx, tx, id, address)
}

// clearLeadCommunicationRecord removes what the engine recorded about a lead.
//
// Split out because two callers need it and a lead's rows are reached by
// lead_id where a contact's are reached by contact_id — sharing
// clearCommunicationRecord would mean one function branching on a subject kind
// its callers already know.
func clearLeadCommunicationRecord(ctx context.Context, tx pgx.Tx, id ids.UUID, address *string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM communication_basis WHERE lead_id = $1`, id); err != nil {
		return fmt.Errorf("clear the lead's communication bases: %w", err)
	}
	// A SUPPRESSION IS DETACHED, NOT DELETED, exactly as the contact arm detaches
	// it. An anonymized subject may lawfully come back — re-captured from the
	// same mailbox — and an objection deleted with them means they come back
	// mailable, having never withdrawn it. Deleting is what an ERASURE does,
	// and erasure writes an erasure_suppression hash to keep the refusal;
	// retention anonymize writes none, so the address is all that is left to
	// carry it.
	if address != nil && *address != "" {
		if _, err := tx.Exec(ctx, `
			UPDATE communication_suppression
			   SET lead_id = NULL, address = coalesce(address, $2)
			 WHERE lead_id = $1`, id, *address); err != nil {
			return fmt.Errorf("detach the lead's suppressions onto their address: %w", err)
		}
	}
	// Whatever the detach could not reach names a lead who is going.
	if _, err := tx.Exec(ctx,
		`DELETE FROM communication_suppression WHERE lead_id = $1`, id); err != nil {
		return fmt.Errorf("clear the lead's suppressions: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE communication_decision
		   SET recipient_address = 'erased+' || id || '@example.invalid',
		       subject_id = NULL, subject_kind = NULL
		 WHERE subject_kind = 'lead' AND subject_id = $1`, id); err != nil {
		return fmt.Errorf("clear the lead's decisions: %w", err)
	}
	return nil
}
