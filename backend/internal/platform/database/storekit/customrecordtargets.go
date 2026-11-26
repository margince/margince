// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CustomFieldTargetMissing is the code a record field answers with when its
// record does not exist, is archived, or is not readable by the caller.
const CustomFieldTargetMissing = "record_not_found"

// EnsureRecordTargets refuses a record-typed custom field whose value names no
// live record of the field's kind that the caller may read. A value of the wrong
// shape is left for CustomBindValues, which every write runs as well. The probe
// is the one a link target takes, so an archive cannot commit before the write.
//
//craft:ignore naked-any values is a request body's extension map
func EnsureRecordTargets(ctx context.Context, tx pgx.Tx, active []fieldcatalog.Column, values map[string]any) error {
	var refused CustomFieldErrors
	for _, c := range active {
		raw, ok := values[c.Name].(string)
		if c.Type != fieldcatalog.TypeRecord || !ok || len(c.Options) != 1 {
			continue
		}
		id, err := ids.Parse(raw)
		if err != nil {
			continue
		}
		err = apperrors.ErrNotFound
		if auth.Allows(ctx, c.Options[0], principal.ActionRead) {
			err = auth.EnsureLinkTarget(ctx, tx, c.Options[0], id)
		}
		switch {
		case errors.Is(err, apperrors.ErrNotFound):
			refused = append(refused, CustomFieldError{Field: c.Name, Code: CustomFieldTargetMissing, Message: "names no live " + c.Options[0] + " record you can read"})
		case err != nil:
			return err
		}
	}
	if len(refused) > 0 {
		return refused
	}
	return nil
}

// InsertFragmentsIn is InsertFragments for a write that holds a transaction,
// which also refuses a record field naming no live record.
//
//craft:ignore naked-any rawExtra is a request body's extension map
func InsertFragmentsIn(ctx context.Context, tx pgx.Tx, active []fieldcatalog.Column, rawExtra map[string]any, base []any) (cols, placeholders string, args []any, err error) {
	if err = EnsureRecordTargets(ctx, tx, active, rawExtra); err != nil {
		return "", "", nil, err
	}
	return InsertFragments(active, rawExtra, base)
}

// SetCustomFieldPatchIn is SetCustomFieldPatch for a write that holds a
// transaction, which also refuses a record field naming no live record.
//
//craft:ignore naked-any updates and current are a request body's extension map
func SetCustomFieldPatchIn(ctx context.Context, tx pgx.Tx, p *Patch, active []fieldcatalog.Column, updates, current map[string]any) error {
	if err := EnsureRecordTargets(ctx, tx, active, updates); err != nil {
		return err
	}
	return SetCustomFieldPatch(p, active, updates, current)
}
