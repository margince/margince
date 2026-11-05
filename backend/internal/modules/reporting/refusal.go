// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// fieldRefusal refuses one field of a write. It unwraps to ErrInvalidArgument,
// so a caller that branches on the sentinel still sees it. It implements
// FieldFault, so every surface answers 422 naming the field, as the contract
// lists for the analytics writes.
type fieldRefusal struct{ field, code, message string }

func (e *fieldRefusal) Error() string { return e.message }
func (e *fieldRefusal) Unwrap() error { return apperrors.ErrInvalidArgument }
func (e *fieldRefusal) FieldFault() (field, code, message string) {
	return e.field, e.code, e.message
}

func refuse(field, code, message string) error {
	return &fieldRefusal{field: field, code: code, message: message}
}

// refusedField names the request field behind a refusal that a shared helper
// raised without one, and keeps the helper's own sentence. Any other failure
// is the server's and keeps its own answer.
func refusedField(field string, err error) error {
	if _, named := errors.AsType[*fieldRefusal](err); named || !errors.Is(err, apperrors.ErrInvalidArgument) {
		return err
	}
	return refuse(field, "invalid", strings.TrimSuffix(err.Error(), ": "+apperrors.ErrInvalidArgument.Error()))
}
