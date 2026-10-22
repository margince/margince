// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/shared/apperrors"

// RequestAcceptanceFieldError identifies a request/task pairing the caller can correct.
type RequestAcceptanceFieldError struct {
	Field   string
	Message string
}

func (e *RequestAcceptanceFieldError) Error() string { return e.Message }

// FieldFault names the correctable acceptance field for HTTP and tool callers.
func (e *RequestAcceptanceFieldError) FieldFault() (field, code, message string) {
	return e.Field, "invalid_request_acceptance", e.Message
}

type requestHeldError struct{}

func (*requestHeldError) Error() string {
	return "This request cannot be accepted here. Refresh the request before trying again."
}
func (*requestHeldError) Unwrap() error                    { return apperrors.ErrConflict }
func (e *requestHeldError) MessageFault() (string, string) { return "conflict", e.Error() }
