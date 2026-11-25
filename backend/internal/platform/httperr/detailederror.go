// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import "fmt"

// FieldError is one per-field refusal inside a validation fault: which input
// the caller got wrong, the contract's stable code for it, and what to fix.
// It is typed rather than left in the Details bag because it is the ACTIONABLE
// half of a 422 — the surfaces render it (REST into the problem body, MCP into
// the sentence an agent reads), and a surface cannot render what it has to
// guess the shape of.
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// Max is the limit a too_long refusal names, so a screen can say the
	// number in the reader's language. Zero is "not stated".
	Max int `json:"max,omitempty"`
	// Params are the other values the code's sentence names, as text.
	Params map[string]string `json:"params,omitempty"`
}

// DetailedError carries a non-sentinel wire shape: validation errors
// (422 with field errors), duplicate conflicts (409 with existing_id),
// auth failures. Constructed by handlers, mapped here.
type DetailedError struct {
	Status  int
	Code    string
	Detail  string
	Details map[string]any

	// Fields is the per-field breakdown of a validation refusal. It is the
	// single source for that breakdown: the wire body is rendered FROM it, so
	// a surface reading Fields and a client reading the body can never be
	// looking at two different lists.
	Fields []FieldError

	// RetryAfterSeconds, when positive, is rendered as a Retry-After header.
	RetryAfterSeconds int
}

func (e *DetailedError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Detail) }
