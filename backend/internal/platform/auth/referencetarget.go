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
// A missing record and one hidden by row scope answer alike, so the status
// tells nobody which rows exist. The field is what a bare not-found lacks on a
// body that carries several ids.
//
// The probe's not-found stays as the cause. An import row or an agent tool
// still classifies it as not-found, because httperr reads the field first.
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
// It is for a request body that names a record the write points at. A path id
// keeps the bare not-found, because the caller addressed that record and has no
// sibling field to confuse it with.
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
