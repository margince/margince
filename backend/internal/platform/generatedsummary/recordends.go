// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package generatedsummary

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ForgetContacts deletes what was written about these contacts, for all
// readers. An erased or merged contact keeps its row, so no cascade fires.
// Every task explanation goes too: any of them may quote these contacts, and
// each is rebuilt on its next open. So do the status lines that may quote them.
func ForgetContacts(ctx context.Context, tx pgx.Tx, contacts ...ids.ContactID) error {
	keys := make([]ids.UUID, len(contacts))
	for i, contact := range contacts {
		keys[i] = contact.UUID
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM generated_summary WHERE contact_id = ANY($1)`, keys); err != nil {
		return fmt.Errorf("delete the generated summaries of a contact: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM generated_summary WHERE kind = $1`, KindTaskExplanation); err != nil {
		return fmt.Errorf("delete the task explanations that may quote a contact: %w", err)
	}
	return forgetStandingLinesTx(ctx, tx, keys)
}

// CarryCompany moves the summaries of a merged-away company to the survivor.
//
// A reader who holds the same kind on both companies keeps the survivor's own.
// The merged-away one is deleted, so two readers' answers never blend.
func CarryCompany(ctx context.Context, tx pgx.Tx, source, target ids.CompanyID) error {
	moved := pgx.NamedArgs{"source": source, "target": target}
	if _, err := tx.Exec(ctx, `
		UPDATE generated_summary a SET company_id = @target
		 WHERE a.company_id = @source
		   AND NOT EXISTS (SELECT 1 FROM generated_summary b
		                    WHERE b.company_id = @target
		                      AND b.user_id = a.user_id AND b.kind = a.kind)`, moved); err != nil {
		return fmt.Errorf("carry the generated summaries of a company: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM generated_summary WHERE company_id = @source`, moved); err != nil {
		return fmt.Errorf("delete the merged-away generated summaries of a company: %w", err)
	}
	// The survivor's line is rewritten when its next activity lands.
	if _, err := tx.Exec(ctx, `DELETE FROM standing_line
		 WHERE entity_type = 'company' AND entity_id = ANY(ARRAY[@source, @target]::uuid[])`, moved); err != nil {
		return fmt.Errorf("delete the status lines of merged companies: %w", err)
	}
	return nil
}
