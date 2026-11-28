// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package customfields

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/labeltranslation"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SetOptions replaces a picklist or multiselect field's allowed option set and
// regenerates the physical column's CHECK constraint from it
// (CUSTOM-FIELDS-PARAM-5). Besides Create it is the one lifecycle mutation that
// runs DDL, so it has the same one-transaction schema-pool shape. It clears
// removed choices off the records, runs the owner ALTER, then writes the
// catalog UPDATE and one audit row. All of it runs as the schema pool's owner role. The
// answer's RecordsChanged counts the records that lost a choice.
func (s *Service) SetOptions(ctx context.Context, id ids.UUID, options []string) (crmcontracts.CustomField, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return crmcontracts.CustomField{}, err
	}
	if len(options) == 0 {
		return crmcontracts.CustomField{}, ErrLastOption
	}
	for _, o := range options {
		if !validOptionText(o) {
			return crmcontracts.CustomField{}, &ValidationError{Errors: []FieldError{{Field: fieldOptions, Code: codeInvalidCharacters}}}
		}
	}
	if s.schemaPool == nil {
		return crmcontracts.CustomField{}, ErrSchemaChangesUnavailable
	}

	// pool-exempt: DDL runs as the schema owner, so the owner pool opens the transaction; the catalog row and its audit row commit in it.
	tx, err := s.schemaPool.Begin(ctx)
	if err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: begin schema tx: %w", err)
	}
	// Error-path safety net only. After Commit this is pgx's ErrTxClosed
	// no-op, and on the error path the caller must see the operation's own
	// error (the WithTx spelling).
	//craft:ignore swallowed-errors deferred rollback of a committed tx is a designed no-op; real failures already left through the operation error
	defer func() { _ = tx.Rollback(ctx) }()

	out, err := s.setOptionsInTx(ctx, tx, id, options)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: commit schema tx: %w", err)
	}
	return out, nil
}

// lockPicklistField takes FOR UPDATE on the catalog row and refuses a
// non-picklist or retired target. The type check answers first — type is
// the operation's precondition; the status gate then freezes a retired
// field's options (mutable).
func lockPicklistField(ctx context.Context, tx pgx.Tx, id ids.UUID) (lockedField, error) {
	var f lockedField
	err := tx.QueryRow(ctx,
		`SELECT object, column_name, type, status, label, options, version, option_translations
		   FROM custom_field WHERE id = $1 FOR UPDATE`,
		id).Scan(&f.Object, &f.ColumnName, &f.Type, &f.Status, &f.Label, &f.OptionsRaw, &f.Version, &f.OptionTranslations)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedField{}, apperrors.ErrNotFound
	}
	if err != nil {
		return lockedField{}, fmt.Errorf("customfields: locking catalog row: %w", err)
	}
	if !hasOptions(f.Type) {
		return lockedField{}, ErrNotPicklist
	}
	if err := f.mutable(); err != nil {
		return lockedField{}, err
	}
	return f, nil
}

// setOptionsInTx is SetOptions' transaction body: row lock →
// picklist check → advisory lock → CHECK regeneration → catalog UPDATE +
// audit, all as the schema pool's owner role.
func (s *Service) setOptionsInTx(ctx context.Context, tx pgx.Tx, id ids.UUID, options []string) (crmcontracts.CustomField, error) {
	f, err := lockPicklistField(ctx, tx, id)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	ddl, err := buildTypedOptionsDDL(f.Object, f.ColumnName, f.Type, options)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	oldOptions, err := unmarshalOptions(f.OptionsRaw)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	// The stored list sent again, in its order, needs no DDL and no history.
	if slices.Equal(oldOptions, options) {
		out, err := readField(ctx, tx, id)
		out.RecordsChanged = new(0)
		return out, err
	}

	// Same arming as Create (bounded lock waits + one ALTER per core
	// table at a time). Lock order is row-then-advisory in every flow
	// that holds both, so the two DDL paths cannot deadlock each other.
	if err := serializeSchemaChange(ctx, tx, f.Object); err != nil {
		return crmcontracts.CustomField{}, err
	}
	if err := lockForTheAlter(ctx, tx, f.Object); err != nil {
		return crmcontracts.CustomField{}, err
	}
	changed, err := s.clearRemovedChoices(ctx, tx, f, removedChoices(oldOptions, options))
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	if _, err := tx.Exec(ctx, ddl); err != nil {
		if _, ok := storekit.CheckViolation(err); ok {
			// ADD CONSTRAINT validates existing rows. Only an object no
			// remover clears can still hold a removed choice here.
			return crmcontracts.CustomField{}, fmt.Errorf(
				"existing %s values still use a removed option; migrate them first: %w", f.Object, apperrors.ErrConflict)
		}
		if lockTimedOut(err) {
			return crmcontracts.CustomField{}, ErrTableBusy
		}
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: regenerating options CHECK: %w", err)
	}

	optionsArg, err := marshalOptions(options)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE custom_field SET options = $1,
		        option_translations = COALESCE((SELECT jsonb_object_agg(key, value)
		                                          FROM jsonb_each(option_translations)
		                                         WHERE key = ANY ($3::text[])), '{}'::jsonb)
		  WHERE id = $2`, optionsArg, id, options); err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: updating options: %w", err)
	}
	forgetActiveColumns(tx, f.Object)
	out, err := readField(ctx, tx, id)
	if err != nil {
		return crmcontracts.CustomField{}, err
	}
	out.RecordsChanged = &changed

	if _, err := storekit.Audit(ctx, tx, "update", rbacObject, id,
		map[string]any{fieldOptions: oldOptions, fieldOptionTranslations: f.OptionTranslations},
		map[string]any{fieldOptions: options, fieldOptionTranslations: labeltranslation.OptionsFromWire(out.OptionTranslations)}); err != nil {
		return crmcontracts.CustomField{}, fmt.Errorf("customfields: audit write: %w", err)
	}
	return out, nil
}
