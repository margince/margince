// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Taking back one technical lookup: the module's own inverse of
// ApplyTechnicalEnrichmentTx.
//
// The lookup writes company_fact rows and no company column, so its audit row
// has no before-image. Its receipt carries the fact rows before and after,
// under a format number. Undo puts them back only while the company and its
// facts stand as the lookup left them, so it never overwrites later work.

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// technicalRestoreFormat is the one snapshot layout this build can put back.
// A receipt with any other number, or none, is refused rather than replayed
// against a table that may have changed shape since it was written.
const technicalRestoreFormat = 1

// technicalRestoreKey is where the snapshot sits inside the receipt's
// "technical" evidence.
const technicalRestoreKey = "restore"

// technicalRestoreSource marks the audit row of an undone lookup.
const technicalRestoreSource = "technical_restore"

// ErrTechnicalReceiptLegacy refuses a lookup receipt written before receipts
// kept the replaced rows. Nothing can say what those rows were.
var ErrTechnicalReceiptLegacy = fmt.Errorf("this lookup was recorded without the facts it replaced: %w", apperrors.ErrConflict)

// ErrTechnicalLaterChange refuses a lookup the company or its facts moved past.
var ErrTechnicalLaterChange = fmt.Errorf("the company or its facts changed after this lookup: %w", apperrors.ErrConflict)

// technicalFactRow is one company_fact row, column by column. Explicit fields
// rather than to_jsonb, so a later column cannot ride a replay unseen.
type technicalFactRow struct {
	ID              ids.UUID   `json:"id"`
	Category        string     `json:"category"`
	Field           string     `json:"field"`
	Value           string     `json:"value"`
	ValueKey        string     `json:"value_key"`
	EvidenceSnippet *string    `json:"evidence_snippet"`
	SourceURL       *string    `json:"source_url"`
	Confidence      *float32   `json:"confidence"`
	Source          string     `json:"source"`
	CapturedBy      string     `json:"captured_by"`
	SiteReadID      *ids.UUID  `json:"site_read_id"`
	CapturedAt      time.Time  `json:"captured_at"`
	Version         int64      `json:"version"`
	RetrievedAt     *time.Time `json:"retrieved_at"`
	VerifiedAt      *time.Time `json:"verified_at"`
	VerifiedBy      *ids.UUID  `json:"verified_by"`
}

type technicalRestore struct {
	Format int                `json:"format"`
	Fields []string           `json:"fields"`
	Before []technicalFactRow `json:"before"`
	After  []technicalFactRow `json:"after"`
}

// IsTechnicalReceipt reports whether an audit row's evidence is a technical
// lookup's, whatever its age.
func IsTechnicalReceipt(evidence json.RawMessage) bool {
	var receipt struct {
		Source string `json:"source"`
	}
	return json.Unmarshal(evidence, &receipt) == nil && receipt.Source == companySourceTechnical
}

func technicalRestoreOf(evidence json.RawMessage) (technicalRestore, error) {
	var receipt struct {
		Technical struct {
			Restore *technicalRestore `json:"restore"`
		} `json:"technical"`
	}
	if json.Unmarshal(evidence, &receipt) != nil || receipt.Technical.Restore == nil ||
		receipt.Technical.Restore.Format != technicalRestoreFormat || len(receipt.Technical.Restore.Fields) == 0 {
		return technicalRestore{}, ErrTechnicalReceiptLegacy
	}
	for _, field := range receipt.Technical.Restore.Fields {
		if LaneOwningField(field) == "" {
			return technicalRestore{}, ErrTechnicalReceiptLegacy
		}
	}
	return *receipt.Technical.Restore, nil
}

// readTechnicalFactRows reads the rows of these fields, locked, in id order.
func readTechnicalFactRows(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, fields []string) ([]technicalFactRow, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, category, field, value, value_key, evidence_snippet, source_url, confidence,
		       source, captured_by, site_read_id, captured_at, version, retrieved_at, verified_at, verified_by
		  FROM company_fact
		 WHERE company_id = $1 AND field = ANY($2)
		 ORDER BY id
		   FOR UPDATE`, companyID, fields)
	if err != nil {
		return nil, fmt.Errorf("read the technical facts: %w", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (technicalFactRow, error) {
		var f technicalFactRow
		err := row.Scan(&f.ID, &f.Category, &f.Field, &f.Value, &f.ValueKey, &f.EvidenceSnippet, &f.SourceURL,
			&f.Confidence, &f.Source, &f.CapturedBy, &f.SiteReadID, &f.CapturedAt, &f.Version,
			&f.RetrievedAt, &f.VerifiedAt, &f.VerifiedBy)
		// UTC, so a snapshot written by one process compares equal to the
		// same row read by another in a different zone.
		f.CapturedAt = f.CapturedAt.UTC()
		f.RetrievedAt, f.VerifiedAt = utcOrNil(f.RetrievedAt), utcOrNil(f.VerifiedAt)
		return f, err
	})
	if err != nil {
		return nil, fmt.Errorf("read the technical facts: %w", err)
	}
	return found, nil
}

// JudgeTechnicalRestoreTx answers whether the lookup recorded at auditID can
// be put back now. The preview and the write both ask it, the write under the
// company lock. A nil answer means it can.
func JudgeTechnicalRestoreTx(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, auditID ids.UUID, at time.Time, evidence json.RawMessage,
) error {
	restore, err := technicalRestoreOf(evidence)
	if err != nil {
		return err
	}
	var later bool
	// The company's own rows, and every fact row of it: a live fact by its id,
	// a removed one by the company its before-image names.
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM audit_log a
		   WHERE (a.occurred_at > $2 OR (a.occurred_at = $2 AND a.id > $3))
		     AND ((a.entity_type = 'company' AND a.entity_id = $1)
		       OR (a.entity_type = 'company_fact'
		           AND (a.entity_id IN (SELECT id FROM company_fact WHERE company_id = $1)
		             OR a.before ->> 'company_id' = $1::text
		             OR a.after ->> 'company_id' = $1::text))))`,
		companyID, at, auditID).Scan(&later)
	if err != nil {
		return fmt.Errorf("read later company changes: %w", err)
	}
	if later {
		return ErrTechnicalLaterChange
	}
	current, err := readTechnicalFactRows(ctx, tx, companyID, restore.Fields)
	if err != nil {
		return err
	}
	if !sameTechnicalRows(current, restore.After) {
		// No audited write explains the difference, so some writer changed
		// facts without one. Refused the same way: the rows are not the
		// lookup's any more.
		return ErrTechnicalLaterChange
	}
	return nil
}

func utcOrNil(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	utc := at.UTC()
	return &utc
}

func sameTechnicalRows(a, b []technicalFactRow) bool {
	return slices.EqualFunc(a, b, func(x, y technicalFactRow) bool {
		left, errLeft := json.Marshal(x)
		right, errRight := json.Marshal(y)
		return errLeft == nil && errRight == nil && string(left) == string(right)
	})
}

// LockTechnicalRestoreTx holds the company for an undo, the order the lookup
// itself takes: company first, then its facts.
func (s *Store) LockTechnicalRestoreTx(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, ifVersion int64) error {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.HoldWritableLive(ctx, tx, entityCompany, companyID.UUID); err != nil {
		return err
	}
	return storekit.EnsureIfMatch(ctx, tx, entityCompany, companyID.UUID, &ifVersion)
}

// RestoreTechnicalFactsTx puts the lookup's before rows back, after
// LockTechnicalRestoreTx and inside the same transaction. `trail` is the
// caller's link from the new audit row to the one it reverses.
func (s *Store) RestoreTechnicalFactsTx(
	ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, auditID ids.UUID, at time.Time,
	evidence json.RawMessage, trail map[string]any,
) error {
	if err := auth.Require(ctx, entityCompany, principal.ActionUpdate); err != nil {
		return err
	}
	if err := JudgeTechnicalRestoreTx(ctx, tx, companyID, auditID, at, evidence); err != nil {
		return err
	}
	restore, err := technicalRestoreOf(evidence)
	if err != nil {
		return err
	}
	if err := replaceTechnicalRows(ctx, tx, companyID, restore); err != nil {
		return err
	}
	auditEvidence := map[string]any{auditKeySource: technicalRestoreSource, auditKeyFields: restore.Fields}
	maps.Copy(auditEvidence, trail)
	auditID, err = storekit.AuditWithEvidence(ctx, tx, "restore", entityCompany, companyID.UUID,
		map[string]any{auditKeyFacts: restore.After}, map[string]any{auditKeyFacts: restore.Before}, auditEvidence)
	if err != nil {
		return fmt.Errorf("audit the technical undo: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, companyID.UUID, crmcontracts.PublicEventCompanyUpdated{
		ChangedFields: map[string]any{auditKeySource: technicalRestoreSource, auditKeyFields: restore.Fields},
	})
}

// replaceTechnicalRows removes what the lookup added and writes back what it
// replaced, under the original ids and provenance. A returned row's version
// moves past both snapshots, so an edit token from before the lookup cannot
// match it again.
func replaceTechnicalRows(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, restore technicalRestore) error {
	after := make([]ids.UUID, 0, len(restore.After))
	for _, row := range restore.After {
		after = append(after, row.ID)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM company_fact WHERE company_id = $1 AND id = ANY($2)`, companyID, after); err != nil {
		return fmt.Errorf("remove the lookup's facts: %w", err)
	}
	for _, row := range restore.Before {
		version := row.Version + 1
		if i := slices.IndexFunc(restore.After, func(a technicalFactRow) bool { return a.ID == row.ID }); i >= 0 {
			version = max(version, restore.After[i].Version+1)
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO company_fact
			       (id, company_id, category, field, value, value_key, evidence_snippet, source_url, confidence,
			        source, captured_by, site_read_id, captured_at, version, retrieved_at, verified_at, verified_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11,
			        (SELECT id FROM site_read WHERE id = $12), $13, $14, $15, $16, $17)`,
			row.ID, companyID, row.Category, row.Field, row.Value, row.ValueKey, row.EvidenceSnippet, row.SourceURL,
			row.Confidence, row.Source, row.CapturedBy, row.SiteReadID, row.CapturedAt, version,
			row.RetrievedAt, row.VerifiedAt, row.VerifiedBy)
		if _, dup := storekit.UniqueViolation(err); dup {
			return fmt.Errorf("a fact now occupies %s: %w", row.Field, ErrTechnicalLaterChange)
		}
		if err != nil {
			return fmt.Errorf("put back fact %s: %w", row.Field, err)
		}
	}
	return nil
}

// technicalRestoreSnapshot is what a lookup records so it can be taken back.
func technicalRestoreSnapshot(fields []string, before, after []technicalFactRow) map[string]any {
	return map[string]any{
		technicalRestoreKey: technicalRestore{
			Format: technicalRestoreFormat, Fields: fields,
			Before: nonNilRows(before), After: nonNilRows(after),
		},
	}
}

func nonNilRows(rows []technicalFactRow) []technicalFactRow {
	if rows == nil {
		return []technicalFactRow{}
	}
	return rows
}

// TechnicalReceiptChange is one field a lookup moved, as its receipt shows it:
// the value before and after, empty where there was none.
type TechnicalReceiptChange struct {
	Field  string `json:"field"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func technicalReceiptChanges(changes []TechnicalChange) []TechnicalReceiptChange {
	receipts := make([]TechnicalReceiptChange, 0, len(changes))
	for _, change := range changes {
		after := change.Value
		if change.Kind == TechnicalGone {
			after = ""
		}
		receipts = append(receipts, TechnicalReceiptChange{Field: change.Field, Before: change.Previous, After: after})
	}
	return receipts
}
