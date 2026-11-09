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
	"reflect"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditKeyProfileFields is the evidence key the receipts travel under. A
// restore records receipts of the rows it put back under the same key, so
// undoing the restore puts them back again.
const auditKeyProfileFields = "profile_fields"

// profileFieldColumns maps each profile field with a column behind it to that
// column's image keys. A receipt is kept only when the entry moved the column,
// because the Undo restores columns and the row is their receipt.
//
// It is derived from the two catalogs that couple a field to a column: the
// read-back's columnBackedColdStartFields and the form's companyFields. The
// display name has no catalog column because the form renames the row itself.
var profileFieldColumns = func() map[string][]string {
	out := map[string][]string{fieldDisplayName: {fieldDisplayName}}
	couple := func(field, column string) {
		if column == columnAddress {
			out[field] = companyAddressColumns
			return
		}
		out[field] = []string{column}
	}
	for field, column := range columnBackedColdStartFields {
		couple(field, column)
	}
	for _, spec := range companyFields {
		if spec.column != "" {
			couple(spec.name, spec.column)
		}
	}
	return out
}()

// profileFieldRow is one profile-field row as an Undo needs it. The content
// tells whether it still stands and is what gets put back; the id and version
// pin the write.
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
	return movedProfileFields(receipts, now)
}

// movedProfileFields compares content, not version, like the column check in
// compose/superseded.go. A row an Undo put back has a new version but holds
// what the earlier entry left. So a walk back through entries is not refused.
func movedProfileFields(receipts map[string]ProfileFieldReceipt, now map[string]profileFieldRow) ([]string, error) {
	var moved []string
	for field, receipt := range receipts {
		current, present := now[field]
		stands := !present && receipt.After == nil
		if present && receipt.After != nil {
			var err error
			if stands, err = sameContent(current.Content, receipt.After.Content); err != nil {
				return nil, err
			}
		}
		if !stands {
			moved = append(moved, field)
		}
	}
	slices.Sort(moved)
	return moved, nil
}

// sameContent compares two row contents as values. Both are jsonb renderings,
// and decoding them keeps a formatting difference from reading as a change.
func sameContent(a, b json.RawMessage) (bool, error) {
	var left, right map[string]any
	if err := json.Unmarshal(a, &left); err != nil {
		return false, fmt.Errorf("contacts: read a profile-field row's content: %w", err)
	}
	if err := json.Unmarshal(b, &right); err != nil {
		return false, fmt.Errorf("contacts: read a profile-field receipt's content: %w", err)
	}
	return reflect.DeepEqual(left, right), nil
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
// It answers receipts of its own writes, so the restore can be undone too.
func restoreProfileFieldsOf(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, trail storekit.AuditTrail) (map[string]ProfileFieldReceipt, error) {
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
	before, err := readProfileFieldImages(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	moved, err := movedProfileFields(receipts, before)
	if err != nil {
		return nil, err
	}
	if len(moved) > 0 {
		return nil, &ProfileFieldMovedError{Fields: moved}
	}
	for _, field := range slices.Sorted(maps.Keys(receipts)) {
		current, present := before[field]
		var pinned *profileFieldRow
		if present {
			pinned = &current
		}
		if err := restoreProfileField(ctx, tx, companyID, field, receipts[field].Before, pinned); err != nil {
			return nil, err
		}
	}
	after, err := readProfileFieldImages(ctx, tx, companyID)
	if err != nil {
		return nil, err
	}
	return restoreReceipts(receipts, before, after), nil
}

// restoreReceipts records, for each row a restore put back, the row as the
// restore found it and as it left it.
func restoreReceipts(undone map[string]ProfileFieldReceipt, before, after map[string]profileFieldRow) map[string]ProfileFieldReceipt {
	out := make(map[string]ProfileFieldReceipt, len(undone))
	for field := range undone {
		var receipt ProfileFieldReceipt
		if row, had := before[field]; had {
			receipt.Before = &row
		}
		if row, has := after[field]; has {
			receipt.After = &row
		}
		out[field] = receipt
	}
	return out
}

// restoreProfileField makes one row hold want, or removes it when want is nil.
// The write is pinned to the current row's id and version, which the caller
// read under the company row lock. A write that matches nothing is a row that
// moved between that read and the write.
func restoreProfileField(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, field string, want, current *profileFieldRow) error {
	var statement string
	var args []any
	switch {
	case want == nil && current != nil:
		statement = `DELETE FROM company_profile_field WHERE id = $1 AND version = $2`
		args = []any{current.ID, current.Version}
	case want != nil && current != nil:
		statement = `
			UPDATE company_profile_field t
			   SET value = r.value, evidence_snippet = r.evidence_snippet, source_url = r.source_url,
			       confidence = r.confidence, source = r.source, captured_by = r.captured_by,
			       captured_at = r.captured_at, retrieved_at = r.retrieved_at,
			       verified_at = r.verified_at, verified_by = r.verified_by
			  FROM jsonb_populate_record(NULL::company_profile_field, $3::jsonb) r
			 WHERE t.id = $1 AND t.version = $2`
		args = []any{current.ID, current.Version, want.Content}
	case want != nil:
		statement = `
			INSERT INTO company_profile_field (company_id, field, value, evidence_snippet, source_url,
			       confidence, source, captured_by, captured_at, retrieved_at, verified_at, verified_by)
			SELECT $1, $2, r.value, r.evidence_snippet, r.source_url, r.confidence, r.source, r.captured_by,
			       r.captured_at, r.retrieved_at, r.verified_at, r.verified_by
			  FROM jsonb_populate_record(NULL::company_profile_field, $3::jsonb) r
			ON CONFLICT (company_id, field) DO NOTHING`
		args = []any{companyID, field, want.Content}
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

// withRestoredProfileFields puts back the rows a restore reverses and records
// their receipts on the restore's own trail.
func withRestoredProfileFields(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, trail storekit.AuditTrail) (storekit.AuditTrail, error) {
	receipts, err := restoreProfileFieldsOf(ctx, tx, companyID, trail)
	if err != nil || len(receipts) == 0 {
		return trail, err
	}
	trail.Evidence = maps.Clone(trail.Evidence)
	trail.Evidence[auditKeyProfileFields] = receipts
	return trail, nil
}
