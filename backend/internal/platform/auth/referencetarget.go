// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ReferenceNotFoundError refuses a request body that names a record the caller
// cannot read: 422, naming the field that carried the id.
//
// A missing record and one hidden by row scope answer identically, so the
// status still tells nobody which rows exist. What it adds over a bare not-found
// is the field, which matters on a body that carries several ids (a deal names a
// pipeline, a stage, a company and a project) where "not found" does not say
// which one to fix.
//
// It keeps the probe's own not-found as its cause. A caller that is not an HTTP
// handler (an import row, an agent tool) still classifies it as not-found, and
// httperr reads the field first, so only the wire answer changes.
type ReferenceNotFoundError struct {
	Field string
	Cause error
}

func (e *ReferenceNotFoundError) Error() string {
	return e.Field + " names a record that does not exist or that you cannot read"
}

// Unwrap hands back the probe's not-found, which may itself be a typed cause
// such as a stage that is gone.
func (e *ReferenceNotFoundError) Unwrap() error { return e.Cause }

// FieldFault names the field the caller sent.
func (e *ReferenceNotFoundError) FieldFault() (field, code, message string) {
	return e.Field, "reference_not_found", e.Error()
}

// NameMissingReference turns a probe's not-found into ReferenceNotFoundError and
// leaves every other answer alone, so a permission denial stays a denial.
//
// It is for a request BODY that names a record the write will point at. A path
// id, or a record an action operates on, keeps the bare not-found: the caller
// chose that record by address and there is no sibling field to confuse it with.
func NameMissingReference(field string, err error) error {
	if errors.Is(err, apperrors.ErrNotFound) {
		return &ReferenceNotFoundError{Field: field, Cause: err}
	}
	return err
}

// EnsureReferenceTarget is EnsureLinkTarget for a body field: the same probe,
// the same lock, with a miss that names the field.
func EnsureReferenceTarget(ctx context.Context, tx pgx.Tx, field, table string, id ids.UUID) error {
	return NameMissingReference(field, EnsureLinkTarget(ctx, tx, table, id))
}
