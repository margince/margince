// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What an administrator may type into a legal entity, and how each value is
// cleaned before it is stored. The create and the patch both read this one
// table, so a field cannot be checked on one path and not the other.

import (
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
	Offering                             LegalEntityOffering
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
	wireCountry             = "country"
	wireDefaultLanguage     = "default_language"
	wireIBAN                = "iban"
	wireBIC                 = "bic"
)

// fields is what an entity stores, field by field. The wire name is the
// column's, except for the address, which the contract nests.
func (in LegalEntityInput) fields() []entityField {
	a := in.Address
	out := []entityField{
		{column: companyNameColumn, value: in.DisplayName, limit: legalEntityNameLimit, anchored: true, required: true},
		{column: fieldLegalName, value: in.LegalName, limit: legalEntityLegalLimit, anchored: true, required: true},
		{column: "address_line1", wire: "address.line1", value: a.Line1, limit: legalEntityNameLimit, anchored: true},
		{column: "address_line2", wire: "address.line2", value: a.Line2, limit: legalEntityNameLimit, anchored: true},
		{column: "address_postal_code", wire: "address.postal_code", value: a.PostalCode, limit: legalEntityNameLimit, anchored: true},
		{column: "address_city", wire: "address.city", value: a.City, limit: legalEntityNameLimit, anchored: true},
		{column: "address_region", wire: "address.region", value: a.Region, limit: legalEntityNameLimit, anchored: true},
		{column: "vat_id", value: in.VatID, limit: legalEntityNameLimit, anchored: true},
		{column: fieldRegisterCourt, value: in.RegisterCourt, limit: legalEntityNameLimit, anchored: true},
		{column: fieldRegisterNumber, value: in.RegisterNumber, limit: legalEntityNameLimit, anchored: true},
		{column: wireCountry, value: in.Country, limit: 2, required: true, clean: cleanEntityCountry},
		{column: wireDefaultLanguage, value: in.DefaultLanguage, limit: 2, clean: cleanEntityLanguage},
		{column: "bank_account_holder", value: in.BankAccountHolder, limit: legalEntityNameLimit},
		{column: wireIBAN, value: in.IBAN, limit: 34, clean: cleanIBAN},
		{column: wireBIC, value: in.BIC, limit: 11, clean: cleanBIC},
		{column: "imprint", value: in.Imprint, limit: legalEntityImprintLimit},
		{column: fieldOfferSummary, value: in.Offering.OfferSummary, limit: legalEntityImprintLimit, anchored: true},
		{column: fieldICP, value: in.Offering.ICP, limit: legalEntityImprintLimit, anchored: true},
		{column: fieldValueProposition, value: in.Offering.ValueProposition, limit: legalEntityImprintLimit, anchored: true},
		{column: fieldUSP, value: in.Offering.USP, limit: legalEntityImprintLimit, anchored: true},
	}
	for i := range out {
		if out[i].wire == "" {
			out[i].wire = out[i].column
		}
	}
	return out
}

// asInput is the entity's current values in the shape a submission takes, so
// a patch compares each field against the table it validates through.
func (e LegalEntity) asInput() LegalEntityInput {
	name := e.DisplayName
	return LegalEntityInput{
		DisplayName: &name, LegalName: e.LegalName, Address: e.Address,
		VatID: e.VatID, RegisterCourt: e.RegisterCourt, RegisterNumber: e.RegisterNumber,
		Country: e.Country, DefaultLanguage: e.DefaultLanguage,
		BankAccountHolder: e.BankAccountHolder, IBAN: e.IBAN, BIC: e.BIC, Imprint: e.Imprint,
		Offering: e.Offering,
	}
}

// cleaned is the value to store for a sent field: nil clears the column. sent
// reports whether the submission named the field at all.
func (f entityField) cleaned(isDefault bool) (value *string, sent bool, err error) {
	if f.value == nil {
		return nil, false, nil
	}
	if isDefault && f.anchored {
		return nil, true, &values.ParseError{
			Field: f.wire, Code: codeFieldOnCompanyForm,
			Message: f.wire + " of the default legal entity is the company's; change it on the company form",
		}
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
	return &text, true, nil
}

var entityLanguageShape = regexp.MustCompile(`^[a-z]{2}$`)

// cleanEntityCountry stores the lower-case spelling installation.country uses.
func cleanEntityCountry(raw string) (string, error) {
	if _, ok := assignedCountry[strings.ToUpper(raw)]; !ok {
		return "", &values.ParseError{
			Field: wireCountry, Code: "invalid_country",
			Message: "country is an ISO 3166-1 alpha-2 code, for example de",
		}
	}
	return strings.ToLower(raw), nil
}

func cleanEntityLanguage(raw string) (string, error) {
	lower := strings.ToLower(raw)
	if !entityLanguageShape.MatchString(lower) {
		return "", &values.ParseError{
			Field: wireDefaultLanguage, Code: "invalid_language",
			Message: "default_language is an ISO 639-1 code, for example de",
		}
	}
	return lower, nil
}

var (
	ibanShape = regexp.MustCompile(`^[A-Z]{2}[0-9]{2}[A-Z0-9]{11,30}$`)
	bicShape  = regexp.MustCompile(`^[A-Z]{6}[A-Z0-9]{2}([A-Z0-9]{3})?$`)
)

// cleanIBAN removes the spaces a reader types. It refuses a number whose check
// digits do not match (ISO 13616 mod 97), so a mistyped digit never reaches an
// offer.
func cleanIBAN(raw string) (string, error) {
	iban := strings.ToUpper(strings.Join(strings.Fields(raw), ""))
	if !ibanShape.MatchString(iban) || !ibanChecksumHolds(iban) {
		return "", &values.ParseError{
			Field: wireIBAN, Code: "invalid_iban",
			Message: "iban is not a valid IBAN; check the digits",
		}
	}
	return iban, nil
}

// ibanChecksumHolds folds the rearranged number into a running remainder, a
// digit at a time, so no value ever grows past three digits.
func ibanChecksumHolds(iban string) bool {
	remainder := 0
	for _, r := range iban[4:] + iban[:4] {
		value := int(r - '0')
		if r >= 'A' && r <= 'Z' {
			value = int(r-'A') + 10
			remainder *= 10
		}
		remainder = (remainder*10 + value) % 97
	}
	return remainder == 1
}

func cleanBIC(raw string) (string, error) {
	bic := strings.ToUpper(strings.ReplaceAll(raw, " ", ""))
	if !bicShape.MatchString(bic) {
		return "", &values.ParseError{
			Field: wireBIC, Code: "invalid_bic",
			Message: "bic is 8 or 11 letters and digits, for example DEUTDEFF",
		}
	}
	return bic, nil
}
