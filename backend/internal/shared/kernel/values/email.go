// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"database/sql/driver"
	"fmt"
	"net/mail"
	"strings"
)

// emailField is the field an address ParseError reports against. One name for all
// three refusals, so a caller matching on it meets the same field whichever refusal
// it got.
const emailField = "email"

// Email is a normalized address: parsed once, stored lowercased — the
// same convention the schema enforces (contact_email_norm/lead_email_norm
// CHECKs), so dedupe by address can never miss on case.
type Email struct{ s string }

// ParseEmail accepts a bare addr-spec (no display name — "Ada <a@b>" is
// a UI artifact, not an address) and returns it lowercased. It refuses
// ErasedEmail, which belongs to the erasure and to no subject.
func ParseEmail(raw string) (Email, error) {
	parsed, err := parseAddress(raw)
	if err != nil {
		return Email{}, err
	}
	if parsed.s == ErasedEmail {
		return Email{}, &ParseError{
			Field: emailField, Code: "email_reserved",
			Message: "that address is reserved for erased records and cannot be stored",
		}
	}
	return parsed, nil
}

// parseAddress is ParseEmail without the reserved-address refusal: the shape of an
// address, which is a different question from whether a record may hold it.
//
// IsReservedAddress asks the shape question — it classifies a DOMAIN, including for
// rows that already hold the erasure's own address — so it cannot go through the
// constructor that refuses that address, or every legacy tombstone would read as an
// ordinary one and the sweeps that skip reserved domains would stop skipping them.
func parseAddress(raw string) (Email, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Email{}, &ParseError{Field: emailField, Code: "email_empty", Message: "an email address is required"}
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || !strings.EqualFold(addr.Address, trimmed) {
		return Email{}, &ParseError{
			Field: emailField, Code: "email_malformed",
			Message: "not a plain email address (user@domain, no display name)",
		}
	}
	return Email{s: strings.ToLower(addr.Address)}, nil
}

func (e Email) String() string { return e.s }
func (e Email) IsZero() bool   { return e.s == "" }

// Domain is the part after the last @ — the company-key derivation input.
func (e Email) Domain() string {
	at := strings.LastIndex(e.s, "@")
	if at < 0 {
		return ""
	}
	return e.s[at+1:]
}

func (e Email) Value() (driver.Value, error) { return e.s, nil }

//craft:ignore naked-any sql.Scanner mandates the any source parameter
func (e *Email) Scan(src any) error {
	switch v := src.(type) {
	case string:
		e.s = v
	case []byte:
		e.s = string(v)
	default:
		return fmt.Errorf("values: cannot scan %T into Email", src)
	}
	return nil
}
