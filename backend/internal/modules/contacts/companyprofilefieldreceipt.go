// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What an apply did to the profile-field rows behind the columns it moved, and
// how an Undo of that apply puts those rows back.
//
// The audit images carry the columns, because the update path writes them back.
// Each changed row travels in the evidence as a receipt: its earlier content,
// and the row version the apply left. A restore puts the rows back in its own
// transaction, and refuses when a row no longer carries that version.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditKeyProfileFields is the evidence key the receipts travel under, and
// auditKeyRestoredProfileFields names on a restore's own entry the rows it put
// back. Two keys, so a restore's entry is never read as receipts of its own.
const (
	auditKeyProfileFields         = "profile_fields"
	auditKeyRestoredProfileFields = "restored_profile_fields"
)

// profileFieldColumns maps each profile field with a column behind it to that
// column's image keys. A receipt is kept only when the entry moved the column,
// because the Undo restores columns and the row is their receipt.
var profileFieldColumns = map[string][]string{
	fieldDisplayName:       {fieldDisplayName},
	fieldLegalName:         {columnLegalName},
	fieldIndustry:          {columnIndustry},
	fieldOfferSummary:      {columnDescription},
	fieldRegisteredAddress: companyAddressColumns,
}

// profileFieldRow is one profile-field row as an Undo needs it. The id and
// version tell whether it still stands; the content is what gets put back.
type profileFieldRow struct {
	ID      ids.UUID        `json:"id"`
	Version int64           `json:"version"`
	Content json.RawMessage `json:"content"`
}

// ProfileFieldReceipt is what one apply did to one profile-field row. Before
// is nil when there was no row; After is nil when the apply removed it.
type ProfileFieldReceipt struct {
	Before *profileFieldRow `json:"before"`
	After  *profileFieldRow `json:"after"`
}

// readProfileFieldImages reads the profile-field rows that have a column behind
// them, keyed by field. Read beside readColdStartColumnImages, before and after
// an apply, so the two describe the same transaction's work.
func readProfileFieldImages(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (map[string]profileFieldRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT field, id, version,
		       jsonb_build_object(
		         'value', value, 'evidence_snippet', evidence_snippet, 'source_url', source_url,
		         'confidence', confidence, 'source', source, 'captured_by', captured_by,
		         'captured_at', captured_at, 'retrieved_at', retrieved_at,
		         'verified_at', verified_at, 'verified_by', verified_by)
		  FROM company_profile_field
		 WHERE company_id = $1 AND field = ANY($2)`,
		companyID, slices.Sorted(maps.Keys(profileFieldColumns)))
	if err != nil {
		return nil, fmt.Errorf("read company profile-field images: %w", err)
	}
	defer rows.Close()
	out := map[string]profileFieldRow{}
	for rows.Next() {
		var field string
		var row profileFieldRow
		if err := rows.Scan(&field, &row.ID, &row.Version, &row.Content); err != nil {
			return nil, fmt.Errorf("read company profile-field images: %w", err)
		}
		out[field] = row
	}
	return out, rows.Err()
}

// profileFieldReceipts pairs the two readings into receipts for the rows that
// moved and whose column the changed image names.
//
//craft:ignore naked-any the changed image is the audit image, keyed by column
func profileFieldReceipts(before, after map[string]profileFieldRow, changed map[string]any) map[string]ProfileFieldReceipt {
	receipts := map[string]ProfileFieldReceipt{}
	for field, columns := range profileFieldColumns {
		if !slices.ContainsFunc(columns, func(c string) bool { _, moved := changed[c]; return moved }) {
			continue
		}
		was, had := before[field]
		now, has := after[field]
		if had == has && (!had || (was.ID == now.ID && was.Version == now.Version)) {
			continue
		}
		var receipt ProfileFieldReceipt
		if had {
			receipt.Before = &was
		}
		if has {
			receipt.After = &now
		}
		receipts[field] = receipt
	}
	return receipts
}

// withProfileFieldReceipts adds the receipts to an entry's evidence. An entry
// that moved no such row carries no key, so its evidence reads as before.
//
//craft:ignore naked-any the audit evidence seam is jsonb
func withProfileFieldReceipts(evidence map[string]any, receipts map[string]ProfileFieldReceipt) map[string]any {
	if len(receipts) > 0 {
		evidence[auditKeyProfileFields] = receipts
	}
	return evidence
}

// profileReceiptsOf reads the receipts an entry's evidence carries.
func profileReceiptsOf(evidence json.RawMessage) (map[string]ProfileFieldReceipt, error) {
	if len(evidence) == 0 {
		return nil, nil
	}
	var said struct {
		Receipts map[string]ProfileFieldReceipt `json:"profile_fields"`
	}
	if err := json.Unmarshal(evidence, &said); err != nil {
		return nil, fmt.Errorf("contacts: read an entry's profile-field receipts: %w", err)
	}
	return said.Receipts, nil
}

// ProfileFieldsMovedSince names the profile fields an entry wrote that no longer
// stand as it left them. The undo evaluator asks it beside its column check.
// The restore asks it again under the company row lock before it writes.
func ProfileFieldsMovedSince(ctx context.Context, tx pgx.Tx, companyID ids.UUID, evidence json.RawMessage) ([]string, error) {
	receipts, err := profileReceiptsOf(evidence)
	if err != nil || len(receipts) == 0 {
		return nil, err
	}
	now, err := readProfileFieldImages(ctx, tx, ids.From[ids.CompanyKind](companyID))
	if err != nil {
		return nil, err
	}
	var moved []string
	for field, receipt := range receipts {
		current, present := now[field]
		switch {
		case receipt.After == nil && present,
			receipt.After != nil && !present,
			receipt.After != nil && (current.ID != receipt.After.ID || current.Version != receipt.After.Version):
			moved = append(moved, field)
		}
	}
	slices.Sort(moved)
	return moved, nil
}

// ProfileFieldMovedError refuses a restore whose profile-field rows were
// written since the entry it reverses. Fields names them.
type ProfileFieldMovedError struct {
	Fields []string
}

func (e *ProfileFieldMovedError) Error() string {
	return fmt.Sprintf("contacts: the profile fields %v were changed since this entry", e.Fields)
}

// restoreProfileFieldsOf puts back the profile-field rows the reversed entry
// moved, on the restore's own transaction. It is a no-op for every write that
// is not a restore, and for a restore of an entry that carried no receipts.
// It returns the fields it put back, for the restore's own evidence.
func restoreProfileFieldsOf(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, trail storekit.AuditTrail) ([]string, error) {
	if trail.Verb != storekit.VerbRestore {
		return nil, nil
	}
	undone, named := trail.Evidence[storekit.EvidenceKeyUndidAuditLog].(string)
	if !named {
		return nil, nil
	}
	var evidence json.RawMessage
	err := tx.QueryRow(ctx, `
		SELECT evidence FROM audit_log
		 WHERE id = $1 AND entity_type = $2 AND entity_id = $3`,
		undone, entityCompany, companyID).Scan(&evidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contacts: read the entry a restore reverses: %w", err)
	}
	receipts, err := profileReceiptsOf(evidence)
	if err != nil || len(receipts) == 0 {
		return nil, err
	}
	moved, err := ProfileFieldsMovedSince(ctx, tx, companyID.UUID, evidence)
	if err != nil {
		return nil, err
	}
	if len(moved) > 0 {
		return nil, &ProfileFieldMovedError{Fields: moved}
	}
	fields := slices.Sorted(maps.Keys(receipts))
	for _, field := range fields {
		if err := restoreProfileField(ctx, tx, companyID, field, receipts[field]); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

// restoreProfileField puts one row back as the receipt's before side names it.
// Each statement is pinned to the row the receipt names. A write that matches
// nothing is a row that moved between the check and the write.
func restoreProfileField(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, field string, receipt ProfileFieldReceipt) error {
	var statement string
	var args []any
	switch {
	case receipt.Before == nil && receipt.After != nil:
		statement = `DELETE FROM company_profile_field WHERE id = $1 AND version = $2`
		args = []any{receipt.After.ID, receipt.After.Version}
	case receipt.Before != nil && receipt.After != nil:
		statement = `
			UPDATE company_profile_field t
			   SET value = r.value, evidence_snippet = r.evidence_snippet, source_url = r.source_url,
			       confidence = r.confidence, source = r.source, captured_by = r.captured_by,
			       captured_at = r.captured_at, retrieved_at = r.retrieved_at,
			       verified_at = r.verified_at, verified_by = r.verified_by
			  FROM jsonb_populate_record(NULL::company_profile_field, $3::jsonb) r
			 WHERE t.id = $1 AND t.version = $2`
		args = []any{receipt.After.ID, receipt.After.Version, receipt.Before.Content}
	case receipt.Before != nil:
		statement = `
			INSERT INTO company_profile_field (company_id, field, value, evidence_snippet, source_url,
			       confidence, source, captured_by, captured_at, retrieved_at, verified_at, verified_by)
			SELECT $1, $2, r.value, r.evidence_snippet, r.source_url, r.confidence, r.source, r.captured_by,
			       r.captured_at, r.retrieved_at, r.verified_at, r.verified_by
			  FROM jsonb_populate_record(NULL::company_profile_field, $3::jsonb) r
			ON CONFLICT (company_id, field) DO NOTHING`
		args = []any{companyID, field, receipt.Before.Content}
	default:
		return nil
	}
	tag, err := tx.Exec(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("contacts: put back the %s profile field: %w", field, err)
	}
	if tag.RowsAffected() != 1 {
		return &ProfileFieldMovedError{Fields: []string{field}}
	}
	return nil
}

// withRestoredProfileFields puts back the rows a restore reverses and names
// them on the restore's own trail.
func withRestoredProfileFields(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, trail storekit.AuditTrail) (storekit.AuditTrail, error) {
	restored, err := restoreProfileFieldsOf(ctx, tx, companyID, trail)
	if err != nil || len(restored) == 0 {
		return trail, err
	}
	trail.Evidence = maps.Clone(trail.Evidence)
	trail.Evidence[auditKeyRestoredProfileFields] = restored
	return trail, nil
}
