// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

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
