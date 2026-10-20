// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What an administrator may type into a legal entity, and how each value is
// cleaned before it is stored. The create and the patch both read this one
// table, so a field cannot be checked on one path and not the other.

import (
	"math/big"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// LegalEntityInput is one submission. A nil field was not sent; on a patch it
// keeps its value. A field sent empty is cleared.
type LegalEntityInput struct {
	DisplayName, LegalName               *string
	Address                              LegalEntityAddress
	VatID, RegisterCourt, RegisterNumber *string
	Country, DefaultLanguage             *string
	BankAccountHolder, IBAN, BIC         *string
	Imprint                              *string
}

// entityField is one column with the value a submission sent for it.
type entityField struct {
	column string
	// wire is the contract's name for the field, which a refusal points at.
	wire  string
	value *string
	limit int
	// anchored fields are read from the anchor company on the default entity,
	// so the default entity refuses them.
	anchored bool
	// required fields are never empty on an entity that is not the default.
	required bool
	clean    func(string) (string, error)
}

const (
	codeTooLong             = "too_long"
	codeFieldOnCompanyForm  = "legal_entity_field_on_company_form"
	legalEntityNameLimit    = 200
	legalEntityLegalLimit   = 300
	legalEntityImprintLimit = 4000
)

func (in LegalEntityInput) fields() []entityField {
	a := in.Address
	return []entityField{
		{column: "display_name", wire: "display_name", value: in.DisplayName, limit: legalEntityNameLimit, anchored: true, required: true},
		{column: "legal_name", wire: "legal_name", value: in.LegalName, limit: legalEntityLegalLimit, anchored: true, required: true},
		{column: "address_line1", wire: "address.line1", value: a.Line1, limit: legalEntityNameLimit, anchored: true},
		{column: "address_line2", wire: "address.line2", value: a.Line2, limit: legalEntityNameLimit, anchored: true},
		{column: "address_postal_code", wire: "address.postal_code", value: a.PostalCode, limit: legalEntityNameLimit, anchored: true},
		{column: "address_city", wire: "address.city", value: a.City, limit: legalEntityNameLimit, anchored: true},
		{column: "address_region", wire: "address.region", value: a.Region, limit: legalEntityNameLimit, anchored: true},
		{column: "vat_id", wire: "vat_id", value: in.VatID, limit: legalEntityNameLimit, anchored: true},
		{column: "register_court", wire: "register_court", value: in.RegisterCourt, limit: legalEntityNameLimit, anchored: true},
		{column: "register_number", wire: "register_number", value: in.RegisterNumber, limit: legalEntityNameLimit, anchored: true},
		{column: "country", wire: "country", value: in.Country, limit: 2, required: true, clean: cleanEntityCountry},
		{column: "default_language", wire: "default_language", value: in.DefaultLanguage, limit: 2, clean: cleanEntityLanguage},
		{column: "bank_account_holder", wire: "bank_account_holder", value: in.BankAccountHolder, limit: legalEntityNameLimit},
		{column: "iban", wire: "iban", value: in.IBAN, limit: 34, clean: cleanIBAN},
		{column: "bic", wire: "bic", value: in.BIC, limit: 11, clean: cleanBIC},
		{column: "imprint", wire: "imprint", value: in.Imprint, limit: legalEntityImprintLimit},
	}
}

// cleaned is the value to store for a sent field: nil clears the column. sent
// reports whether the submission named the field at all.
func (f entityField) cleaned(isDefault bool) (value any, sent bool, err error) {
	if f.value == nil {
		return nil, false, nil
	}
	if isDefault && f.anchored {
		return nil, true, &values.ParseError{Field: f.wire, Code: codeFieldOnCompanyForm,
			Message: f.wire + " of the default legal entity is the company's; change it on the company form"}
	}
	text := strings.TrimSpace(*f.value)
	if text == "" {
		if f.required && !isDefault {
			return nil, true, &values.ParseError{Field: f.wire, Code: codeRequired, Message: f.wire + " is required"}
		}
		return nil, true, nil
	}
	if f.clean != nil {
		if text, err = f.clean(text); err != nil {
			return nil, true, err
		}
	}
	if utf8.RuneCountInString(text) > f.limit {
		return nil, true, &values.ParseError{Field: f.wire, Code: codeTooLong, Message: f.wire + " is too long"}
	}
	return text, true, nil
}

var entityLanguageShape = regexp.MustCompile(`^[a-z]{2}$`)

// cleanEntityCountry stores the lower-case spelling installation.country uses.
func cleanEntityCountry(raw string) (string, error) {
	if _, ok := assignedCountry[strings.ToUpper(raw)]; !ok {
		return "", &values.ParseError{Field: "country", Code: "invalid_country",
			Message: "country is an ISO 3166-1 alpha-2 code, for example de"}
	}
	return strings.ToLower(raw), nil
}

func cleanEntityLanguage(raw string) (string, error) {
	lower := strings.ToLower(raw)
	if !entityLanguageShape.MatchString(lower) {
		return "", &values.ParseError{Field: "default_language", Code: "invalid_language",
			Message: "default_language is an ISO 639-1 code, for example de"}
	}
	return lower, nil
}

var (
	ibanShape = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
	bicShape  = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
)

// cleanIBAN removes the spaces people type and refuses a number whose check
// digits do not match (ISO 13616 mod 97), which catches a mistyped digit
// before it reaches an offer.
func cleanIBAN(raw string) (string, error) {
	iban := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if !ibanShape.MatchString(iban) || !ibanChecksumHolds(iban) {
		return "", &values.ParseError{Field: "iban", Code: "invalid_iban",
			Message: "iban is not a valid IBAN; check the digits"}
	}
	return iban, nil
}

func ibanChecksumHolds(iban string) bool {
	var digits strings.Builder
	for _, r := range iban[4:] + iban[:4] {
		if r >= 'A' && r <= 'Z' {
			digits.WriteString(big.NewInt(int64(r - 'A' + 10)).String())
			continue
		}
		digits.WriteRune(r)
	}
	n, ok := new(big.Int).SetString(digits.String(), 10)
	return ok && new(big.Int).Mod(n, big.NewInt(97)).Int64() == 1
}

func cleanBIC(raw string) (string, error) {
	bic := strings.ToUpper(strings.ReplaceAll(raw, " ", ""))
	if !bicShape.MatchString(bic) {
		return "", &values.ParseError{Field: "bic", Code: "invalid_bic",
			Message: "bic is 8 or 11 letters and digits, for example DEUTDEFF"}
	}
	return bic, nil
}
