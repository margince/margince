// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

// Update and Retire are the two catalog-only lifecycle mutations. Neither
// runs DDL, so both stay on the workspace-bound app pool inside
// database.WithWorkspaceTx. Only Create and SetOptions need the schema
// pool's one-transaction owner-ALTER shape (create.go, options.go).

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lockedField is the FOR UPDATE snapshot that Update, Retire and SetOptions
// decide from. The lock is taken before any decision read, so the snapshot
// cannot go stale under a concurrent writer.
type lockedField struct {
	Object       string
	ColumnName   string
	Type         string
	Status       string
	Label        string
	OptionsRaw   []byte
	ShowInCreate bool
	Reportable   bool
	Version      int64
}

// mutable is the status gate on the two catalog mutations a retired field no
// longer accepts: the settings update (Update) and the options edit
// (lockPicklistField). Retirement is terminal, so the settings and options are
// frozen while the row stays fetchable. The gate is not inside lockField
// because Retire needs the retired row to answer a repeat retire unchanged.
func (f lockedField) mutable() error {
	if f.Status == statusRetired {
		return ErrFieldRetired
	}
	return nil
}

// lockField takes FOR UPDATE on one catalog row. The custom_field catalog
// is installation-wide admin config with no owner_id — the object grant is
// the whole authority question (the pipeline precedent), so there is no
// row-scope probe to add. A missing row answers ErrNotFound.
func lockField(ctx context.Context, tx pgx.Tx, id ids.UUID) (lockedField, error) {
	var f lockedField
	err := tx.QueryRow(ctx,
		`SELECT object, column_name, type, status, label, options, show_in_create, reportable, version
		   FROM custom_field WHERE id = $1 FOR UPDATE`,
		id).Scan(&f.Object, &f.ColumnName, &f.Type, &f.Status, &f.Label, &f.OptionsRaw, &f.ShowInCreate, &f.Reportable, &f.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedField{}, apperrors.ErrNotFound
	}
	if err != nil {
		return lockedField{}, fmt.Errorf("customfields: locking catalog row: %w", err)
	}
	return f, nil
}

// readField re-reads the full row after a mutation, inside the same
// transaction, so the response carries the trigger-bumped version.
func readField(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.CustomField, error) {
	out, err := scanCustomField(tx.QueryRow(ctx,
		`SELECT `+catalogColumns+` FROM custom_field WHERE id = $1`, id))
	if err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: reading catalog row: %w", err)
	}
	return out, nil
}

// FieldPatch is the merge-PATCH of the two settings a live field may change.
// A nil member is left as it is.
type FieldPatch struct {
	Label        *string
	ShowInCreate *bool
	Reportable   *bool
}

// validate refuses a patch that changes nothing or blanks the label. An empty
// patch answers the same label-required refusal a rename always gave, so a
// client that only ever renames sees no new error.
func (p FieldPatch) validate() error {
	if p.ShowInCreate == nil && p.Label == nil && p.Reportable == nil {
		return &ValidationError{Errors: []FieldError{{Field: fieldLabel, Code: codeRequired}}}
	}
	if p.Label != nil && strings.TrimSpace(*p.Label) == "" {
		return &ValidationError{Errors: []FieldError{{Field: fieldLabel, Code: codeRequired}}}
	}
	return nil
}

// Update changes the catalog label, the create-dialog setting, or both
// (CUSTOM-FIELDS-WIRE-3). Slug, column_name, object and type never move, so
// the physical column identity is stable across rename. A retired field
// refuses (ErrFieldRetired, 409). If-Match is the contract's optional CAS: a
// stale version answers ErrVersionSkew, and without one the held row lock
// still serializes concurrent writers.
func (s *Service) Update(ctx context.Context, id ids.UUID, patch FieldPatch, ifVersion *int64) (crmcontracts.CustomField, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return crmcontracts.CustomField{}, err
	}
	if err := patch.validate(); err != nil {
		return crmcontracts.CustomField{}, err
	}

	var out crmcontracts.CustomField
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		f, err := lockField(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := f.mutable(); err != nil {
			return err
		}
		if ifVersion != nil && *ifVersion != f.Version {
			return apperrors.ErrVersionSkew
		}
		next := f
		if patch.Label != nil {
			next.Label = *patch.Label
			if err := serializeSchemaChange(ctx, tx, f.Object); err != nil {
				return err
			}
			if err := refuseTakenLabel(ctx, tx, f.Object, id, next.Label); err != nil {
				return err
			}
		}
		if patch.ShowInCreate != nil {
			next.ShowInCreate = *patch.ShowInCreate
		}
		if patch.Reportable != nil {
			if *patch.Reportable && !reportableType(f.Type) {
				return &ValidationError{Errors: []FieldError{{Field: fieldReport, Code: codeUnsupportedType}}}
			}
			next.Reportable = *patch.Reportable
		}
		if _, err := tx.Exec(ctx,
			`UPDATE custom_field SET label = $1, show_in_create = $2, reportable = $3 WHERE id = $4`,
			next.Label, next.ShowInCreate, next.Reportable, id); err != nil {
			return fmt.Errorf("customfields: updating settings: %w", err)
		}
		if out, err = readField(ctx, tx, id); err != nil {
			return err
		}
		before, after := patch.auditDiff(f, next)
		if _, err := storekit.Audit(ctx, tx, "update", rbacObject, id, before, after); err != nil {
			return fmt.Errorf("customfields: audit write: %w", err)
		}
		return nil
	})
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	return out, nil
}

// auditDiff names only the settings the patch carried, so the trail of a
// rename does not claim the create-dialog setting moved, and the other way.
func (p FieldPatch) auditDiff(was, now lockedField) (before, after map[string]any) {
	before, after = map[string]any{}, map[string]any{}
	if p.Label != nil {
		before[fieldLabel], after[fieldLabel] = was.Label, now.Label
	}
	if p.ShowInCreate != nil {
		before[fieldShow], after[fieldShow] = was.ShowInCreate, now.ShowInCreate
	}
	if p.Reportable != nil {
		before[fieldReport], after[fieldReport] = was.Reportable, now.Reportable
	}
	return before, after
}

// reportableType is the kinds of field a report can group by. One value per
// record keeps a grouped total from counting a record twice, which rules out
// a multiselect; free text and numbers are not categories.
func reportableType(typ string) bool {
	return typ == TypePicklist || typ == TypeBoolean
}

// Retire flips status to retired (CUSTOM-FIELDS-WIRE-4/AC-13): the field
// disappears from record payloads and the sort/filter vocabulary while
// the physical column and every value in it are preserved — never a
// DROP, and archived_at stays null (retire is a status flip on a
// still-fetchable row, not an archive). Retiring an already-retired
// field is a no-op that returns the row unchanged: nothing changed, so
// nothing is written to the audit trail.
func (s *Service) Retire(ctx context.Context, id ids.UUID) (crmcontracts.CustomField, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return crmcontracts.CustomField{}, err
	}

	var out crmcontracts.CustomField
	err := database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		f, err := lockField(ctx, tx, id)
		if err != nil {
			return err
		}
		if f.Status == statusRetired {
			out, err = readField(ctx, tx, id)
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE custom_field SET status = $1 WHERE id = $2`, statusRetired, id); err != nil {
			return fmt.Errorf("customfields: updating status: %w", err)
		}
		if out, err = readField(ctx, tx, id); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", rbacObject, id,
			map[string]any{fieldStatus: f.Status}, map[string]any{fieldStatus: statusRetired}); err != nil {
			return fmt.Errorf("customfields: audit write: %w", err)
		}
		return nil
	})
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	return out, nil
}
