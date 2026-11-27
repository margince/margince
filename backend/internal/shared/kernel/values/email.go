// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import (
	"database/sql/driver"
	"fmt"
	"net/mail"
	"slices"
	"strings"
)

// emailField is the field an address ParseError reports against. One name for every
// refusal, so a caller matching on it meets the same field whichever refusal it got.
const emailField = "email"

// MaxEmailLength is the longest address a mail server accepts on a forward path
// (RFC 5321 §4.5.3.1.3 less the angle brackets), so a longer one cannot be delivered.
const MaxEmailLength = 254

// Email is a normalized address: parsed once, stored lowercased — the
// same convention the schema enforces (contact_email_norm/lead_email_norm
// CHECKs), so dedupe by address can never miss on case.
type Email struct{ s string }

// ParseEmail accepts a bare addr-spec and returns it lowercased; "Ada <a@b.de>"
// is a UI artifact, not an address. It refuses ErasedEmail, which belongs to
// the erasure. It refuses an address no mail can reach: an undotted domain, or
// one past MaxEmailLength.
//
// Scan does not parse, so a row stored before a refusal still reads.
func ParseEmail(raw string) (Email, error) {
	parsed, err := parseAddress(raw)
	if err != nil {
		return Email{}, err
	}
	if len(parsed.s) > MaxEmailLength {
		return Email{}, &ParseError{
			Field: emailField, Code: "too_long", Max: MaxEmailLength,
			Message: fmt.Sprintf("an email address holds at most %d characters", MaxEmailLength),
		}
	}
	if !dottedDomain(parsed.Domain()) {
		return Email{}, &ParseError{
			Field: emailField, Code: "email_malformed",
			Message: "the part after the @ must be a domain such as example.com",
		}
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
	trimmed := identifierText(raw)
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

// dottedDomain holds for two or more non-empty labels; a bare "localhost" is no mail host.
func dottedDomain(domain string) bool {
	labels := strings.Split(domain, ".")
	return len(labels) >= 2 && !slices.Contains(labels, "")
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
