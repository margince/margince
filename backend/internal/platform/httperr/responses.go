// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

// The ready-made refusals: the handful of responses a surface writes directly
// rather than by classifying an error it was handed.
//
// Split from httperr.go at the 500-line cap. The two halves answer different
// questions — that one asks "what does THIS error mean on the wire", this one
// is the shorthand for a refusal a handler already knows the shape of — so the
// seam is a concept rather than a line count.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// Unauthorized is the shared 401.
func Unauthorized(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(w, problem{Status: http.StatusUnauthorized, Code: "unauthorized", Detail: detail})
}

// ServiceUnavailable is the shared 503 for availability states — the
// installation cannot serve (e.g. not yet bootstrapped), which is an
// operator condition, never an authentication failure.
func ServiceUnavailable(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(w, problem{Status: http.StatusServiceUnavailable, Code: "service_unavailable", Detail: detail})
}

// NotImplemented marks a contract operation that exists on the surface
// but has no implementation yet — explicit 501, never a silent 404.
func NotImplemented(w http.ResponseWriter, r *http.Request, op string) {
	writeProblem(w, problem{
		Status: http.StatusNotImplemented,
		Code:   "not_implemented",
		Detail: fmt.Sprintf("operation %s is specified but not yet implemented", op),
	})
}

// Unavailable is ServiceUnavailable with a code of its own.
//
// The code is what a UI can translate. `service_unavailable` says only that
// something is down, so a client with a reader to speak to has to render the
// detail verbatim — and the detail is written in one language, which is how an
// English sentence ends up inside a German screen. A named code lets the client
// carry its own copy and keeps the detail for everyone else: curl, a log, an
// integrator with no catalog.
func Unavailable(w http.ResponseWriter, r *http.Request, code, detail string) {
	writeProblem(w, problem{Status: http.StatusServiceUnavailable, Code: code, Detail: detail})
}

// NotImplementedBecause is NotImplemented for a route that IS implemented and
// cannot serve this installation yet.
//
// Same status, different sentence. 501 covers both "nobody built this" and
// "this deployment has not configured it", and only the first is what the
// generic text describes — so the second one needs to say what is missing, or
// it sends an operator to look for a build that would not help.
func NotImplementedBecause(w http.ResponseWriter, r *http.Request, detail string) {
	writeProblem(w, problem{
		Status: http.StatusNotImplemented,
		Code:   "not_implemented",
		Detail: detail,
	})
}

// Validation is the 422 shape with per-field errors.
func Validation(field, code, message string) *DetailedError {
	return &DetailedError{
		Status: http.StatusUnprocessableEntity,
		Code:   "validation_error",
		Detail: message,
		Fields: []FieldError{{Field: field, Code: code, Message: message}},
	}
}

// RequireNonBlank is the one rule for a record's required name, on create and
// edit alike. It answers the trimmed text, so what was accepted is what is stored.
func RequireNonBlank(field, raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !values.HasVisibleText(name) {
		return "", Validation(field, "required", field+" is required")
	}
	return name, nil
}

// RefuseNull refuses a PATCH that sent a required field as an explicit null.
// An optional pointer decodes null and absent alike, so without this a null
// name reads as "leave it alone" and answers 200 for a clear it never did.
func RefuseNull(r *http.Request, field string) error {
	if slices.Contains(ClearedFields(r), field) {
		return Validation(field, "required", field+" is required")
	}
	return nil
}

// RequireBodyID refuses a required body id the caller simply omitted, naming the
// wire field. Nil when the id is present.
//
// The defect it closes is the generator's: oapi-codegen renders a REQUIRED body
// id as a non-pointer UUID, and encoding/json leaves an absent key at the zero
// value with no error. So "required" in the contract is a claim only this check
// makes true — and what made it worth a named helper is where the zero value
// LANDS. It reaches a lookup or a link-target probe, matches no row, and comes
// back as a bare not-found: the caller is told a record it never mentioned does
// not exist, on a request whose real fault was an absent key.
//
// It lives here rather than per-module because Classify matches *DetailedError
// before anything else, so one call answers a 422 naming the field on REST AND
// the same field-named sentence on the MCP tool surface — which never runs a
// module's HTTP mapper. A module error type per caller would be a second
// spelling of a rule whose wire output is byte-identical.
//
// The id arrives as ids.UUID: this package deliberately does not import the
// generated contracts, so the openapi_types.UUID conversion happens at the call
// site, where the contract type legally lives.
func RequireBodyID(field string, id ids.UUID) error {
	if id.IsZero() {
		return Validation(field, "required", field+" is required")
	}
	return nil
}

// fieldDetails renders the per-field breakdown into the contract's
// `details.errors` body shape. Rendering it here, from Fields, is what keeps
// the typed list and the wire list the same list.
const fieldErrorsKey = "errors"

func fieldDetails(fields []FieldError) map[string]any {
	errs := make([]map[string]string, 0, len(fields))
	for _, f := range fields {
		entry := map[string]string{"field": f.Field, "code": f.Code}
		// A multi-field validator may have no per-entry prose (the code IS the
		// reason). Omitting the key beats shipping "message": "", which reads
		// as an explanation that came out blank.
		if f.Message != "" {
			entry["message"] = f.Message
		}
		errs = append(errs, entry)
	}
	return map[string]any{fieldErrorsKey: errs}
}

// Duplicate is the 409 dedupe shape. existingID is included only when
// known AND disclosable — a conflict with a row outside the caller's
// row scope answers 409 without the id.
func Duplicate(code, existingID string) *DetailedError {
	e := &DetailedError{
		Status: http.StatusConflict,
		Code:   code,
		Detail: "a live record with this key already exists",
	}
	if existingID != "" {
		e.Details = map[string]any{"existing_id": existingID}
	}
	return e
}

// codeInternal is the wire code of every opaque 500.
const codeInternal = "internal"

func writeProblem(w http.ResponseWriter, p problem) {
	if p.Type == "" {
		p.Type = problemTypeBase + p.Code
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	if err := writeEncoded(w, p.Status, "application/problem+json", p); err != nil {
		slog.Error("encoding a problem response; answering it without its details", "code", p.Code, "err", err)
		p.Details = nil
		//craft:ignore swallowed-errors without Details a problem holds only strings and an int, which always encode
		_ = writeEncoded(w, p.Status, "application/problem+json", p)
	}
}

// writeEncoded encodes body in full before the status line goes out. A value
// encoding/json refuses is then an error the caller can still answer, rather
// than a success status followed by no body.
//
//craft:ignore naked-any the JSON serialization seam, shared by WriteJSON and writeProblem
func writeEncoded(w http.ResponseWriter, status int, contentType string, body any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	//craft:ignore swallowed-errors the status line is already on the wire, so a failed write (the client left) has no channel back
	_, _ = w.Write(append(encoded, '\n'))
	return nil
}
