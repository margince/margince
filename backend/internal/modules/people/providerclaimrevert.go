// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Taking a purchase back off the records it filled.
//
// "Delete the bought data" removed the claims and left the values they had
// written standing on the record, which made the control say more than it did:
// an admin was told the purchase was gone while a bought title and profile link
// stayed on every contact.
//
// The whole difficulty is telling OUR value from somebody's later edit. A field
// this filled may since have been corrected by a colleague, and clearing that
// would be destroying a person's work to undo a purchase. So nothing is cleared
// on the strength of "a purchase once wrote here":
//
//   - a plain column is cleared only while it still EQUALS what was written,
//     which is why provider_applied_field keeps the value for those;
//   - a child row is archived only while it still carries the provider as its
//     own source, which is what its continued existence under that source
//     proves.
//
// Either way the test is "is this still ours", asked of the record as it is now
// rather than of the history.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RevertedSubject is one contact the revert reached.
type RevertedSubject struct {
	PersonID ids.UUID
	// Fields are the record fields cleared, by name, for the audit image. Never
	// the values: audit_log outlives the erasure that clears the record.
	Fields []string
}

// SubjectsWithProviderFills names the contacts one provider's purchases have
// written to, so the caller can revert them one transaction at a time.
//
// Read and write are deliberately separate calls. A revert that held every
// affected contact's row in one transaction would be a lock over an unbounded
// set of people — and the eraser takes those rows subject-first, so it is also
// a deadlock against somebody's Art. 17 request.
func SubjectsWithProviderFills(ctx context.Context, tx pgx.Tx, providerName string) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT person_id FROM provider_applied_field WHERE provider = $1`, providerName)
	if err != nil {
		return nil, fmt.Errorf("people: reading whose records a purchase filled: %w", err)
	}
	defer rows.Close()
	var subjects []ids.UUID
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		subjects = append(subjects, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading whose records a purchase filled: %w", err)
	}
	return subjects, nil
}

// RevertProviderFills clears what one provider's purchases put on ONE contact's
// record, and reports which fields went.
//
// EnsureWritable rather than its live twin: this is a data-lifecycle action in
// the same family as the erasure and the retention sweep, and those reach an
// archived record on purpose. A purchase on somebody archived last week is
// exactly what an admin pressing "delete the bought data" means to remove.
func RevertProviderFills(ctx context.Context, tx pgx.Tx, providerName string, subject ids.UUID) (RevertedSubject, error) {
	out := RevertedSubject{PersonID: subject}
	if err := auth.EnsureWritable(ctx, tx, entityPerson, subject); err != nil {
		return out, err
	}
	// IncludeArchived, matching the probe above: the point of this action is
	// reaching a purchase wherever it landed, and an archived contact is where
	// a good deal of it landed.
	if _, err := storekit.LockRow(ctx, tx, entityPerson, subject, storekit.IncludeArchived); err != nil {
		return out, err
	}
	filled, err := appliedFieldsFor(ctx, tx, providerName, subject)
	if err != nil {
		return out, err
	}
	for _, f := range filled {
		cleared, err := revertOne(ctx, tx, f)
		if err != nil {
			return out, err
		}
		if cleared {
			out.Fields = append(out.Fields, f.field)
		}
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM provider_applied_field WHERE provider = $1 AND person_id = $2`,
		providerName, subject); err != nil {
		return out, fmt.Errorf("people: forgetting what a purchase filled: %w", err)
	}
	if len(out.Fields) == 0 {
		return out, nil
	}
	return out, auditReverted(ctx, tx, providerName, out)
}

// appliedFieldsFor reads what one provider filled on one contact.
func appliedFieldsFor(ctx context.Context, tx pgx.Tx, providerName string, subject ids.UUID) ([]appliedField, error) {
	rows, err := tx.Query(ctx, `
		SELECT target_table, target_field, target_row_id, applied_value
		  FROM provider_applied_field
		 WHERE provider = $1 AND person_id = $2`, providerName, subject)
	if err != nil {
		return nil, fmt.Errorf("people: reading what a purchase filled: %w", err)
	}
	defer rows.Close()
	var out []appliedField
	for rows.Next() {
		var f appliedField
		if err := rows.Scan(&f.table, &f.field, &f.rowID, &f.value); err != nil {
			return nil, err
		}
		f.subject, f.provider = subject, providerName
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("people: reading what a purchase filled: %w", err)
	}
	return out, nil
}

// auditReverted names the fields that went. Never their values — the file
// header says why, and it is the same rule the apply side follows.
func auditReverted(ctx context.Context, tx pgx.Tx, providerName string, r RevertedSubject) error {
	before := make(map[string]any, len(r.Fields))
	after := make(map[string]any, len(r.Fields))
	for _, f := range r.Fields {
		before[f] = "bought"
		after[f] = nil
	}
	auditID, err := storekit.AuditWithEvidence(ctx, tx, "update", entityPerson, r.PersonID, before, after,
		map[string]any{auditKeyProvider: providerName, "fields": r.Fields})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, r.PersonID,
		crmcontracts.PublicEventPersonUpdated{ChangedFields: after})
}
