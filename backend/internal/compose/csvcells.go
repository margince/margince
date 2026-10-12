// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every cell of a row is checked before anything is written. The dry run and
// the commit both ask here, so a preview cannot promise a row the store refuses.

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// cellProblem is one cell the store would refuse. Target is the mapping
// target; the part writer turns it into the file's own header.
type cellProblem struct {
	Target, Value, Message string
}

// checkRow checks the core cells, then each custom-field cell in target order.
// It answers the filled phone, LinkedIn and custom-field cells in the shape the
// store holds them. The first problem wins.
func checkRow(object string, fields map[string]string, cat importCatalog) (map[string]any, *cellProblem) {
	if problem := unwritableReason(object, fields); problem != nil {
		return nil, problem
	}
	typed, problem := checkParsedCells(object, fields)
	if problem != nil {
		return nil, problem
	}
	for _, target := range slices.Sorted(maps.Keys(fields)) {
		raw := fields[target]
		if !strings.HasPrefix(target, customFieldPrefix) || strings.TrimSpace(raw) == "" {
			continue
		}
		col, active := cat.byName[target]
		if !active {
			// Retired after the run was staged: the store has no column to bind it to.
			return nil, &cellProblem{
				Target: target, Value: raw,
				Message: fmt.Sprintf("%q cannot be written: this field is no longer active", strings.TrimSpace(raw)),
			}
		}
		value, problem := checkCell(target, col, raw, cat.optionsReadable)
		if problem != nil {
			return nil, problem
		}
		if typed == nil {
			typed = map[string]any{}
		}
		typed[target] = value
	}
	return typed, nil
}

// checkParsedCells reads the phone and LinkedIn cells through the parsers the
// store writes with, so the row carries the spelling the store will hold.
func checkParsedCells(object string, fields map[string]string) (map[string]any, *cellProblem) {
	var typed map[string]any
	for _, target := range []string{fieldPhone, fieldLinkedIn} {
		text := strings.TrimSpace(fields[target])
		if text == "" {
			continue
		}
		stored, err := storedSpelling(object, target, text)
		if err != nil {
			return nil, &cellProblem{Target: target, Value: fields[target], Message: parseMessage(text, err)}
		}
		if typed == nil {
			typed = map[string]any{}
		}
		typed[target] = stored
	}
	return typed, nil
}

// storedSpelling is the parser each object's store runs on a phone or a
// LinkedIn URL. A company's page and a contact's slot are narrower than a
// lead's profile URL.
func storedSpelling(object, target, text string) (string, error) {
	switch {
	case target == fieldPhone:
		phone, err := values.ParsePhone(text)
		return phone.String(), err
	case object == migration.ObjectCompany:
		return contacts.NormalizeCompanyLinkedInURL(text)
	case object == migration.ObjectContact:
		return contacts.NormalizeContactLinkedInURL(text)
	default:
		return contacts.NormalizeLinkedInURL(text)
	}
}

func parseMessage(text string, err error) string {
	parseErr, ok := errors.AsType[*values.ParseError](err)
	if !ok {
		return fmt.Sprintf("%q cannot be read", text)
	}
	if parseErr.Code == "phone_needs_country_code" {
		return fmt.Sprintf("%q needs its country code, like +49 30 1234567", text)
	}
	return fmt.Sprintf("%q: %s", text, parseErr.Message)
}

// checkCell turns one custom-field cell into the value storekit.SQLValue takes.
// SQLValue decides whether it binds, so the dry run accepts the text the store
// does. A refusal lists a picklist's options only
// when the caller may read them (custom_field:read).
//
//craft:ignore naked-any the value is SQLValue's own input, whose shape depends on the column type
func checkCell(target string, col fieldcatalog.Column, raw string, optionsReadable bool) (any, *cellProblem) {
	text := strings.TrimSpace(raw)
	var wire any = text
	message := ""
	switch col.Type {
	case fieldcatalog.TypePicklist:
		option, refusal := picklistValue(col, text, optionsReadable)
		wire, message = option, refusal
	case fieldcatalog.TypeMultiselect:
		chosen, refusal := multiselectValue(col, text, optionsReadable)
		wire, message = chosen, refusal
	case fieldcatalog.TypeCurrency:
		minor, refusal := currencyValue(col, text)
		wire, message = minor, refusal
	case fieldcatalog.TypeBoolean:
		value, refusal := booleanValue(text)
		wire, message = value, refusal
	}
	if message == "" {
		if _, binds := storekit.SQLValue(col, wire); !binds {
			message = unboundMessage(col, text)
		}
	}
	if message != "" {
		return nil, &cellProblem{Target: target, Value: raw, Message: message}
	}
	return wire, nil
}

// picklistValue is the catalog's spelling of the option the text names.
func picklistValue(col fieldcatalog.Column, text string, optionsReadable bool) (string, string) {
	if option, found := optionNamed(col.Options, text); found {
		return option, ""
	}
	return "", notAllowedMessage(col, text, optionsReadable)
}

// multiselectValue splits on semicolons and names each option in the catalog's
// spelling. A comma with no semicolon is the usual mistake, so it gets its own
// sentence once the whole text is not itself an option.
func multiselectValue(col fieldcatalog.Column, text string, optionsReadable bool) ([]string, string) {
	var chosen []string
	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		option, found := optionNamed(col.Options, part)
		if !found {
			if strings.Contains(text, ",") && !strings.Contains(text, ";") {
				return nil, fmt.Sprintf("%q: separate several values with semicolons", text)
			}
			return nil, notAllowedMessage(col, part, optionsReadable)
		}
		chosen = append(chosen, option)
	}
	return chosen, ""
}

// currencyValue is the amount in the column's minor units. A column with no
// currency has no unit to count, so its values cannot be read at all.
func currencyValue(col fieldcatalog.Column, text string) (float64, string) {
	if col.Currency == "" {
		return 0, fmt.Sprintf("%q cannot be read: this currency field has no currency set", text)
	}
	minor, ok := values.MinorUnits(text, col.Currency)
	if !ok {
		return 0, fmt.Sprintf("%q is not an amount in %s; write it like 12000.50", text, col.Currency)
	}
	return float64(minor), ""
}

// spreadsheetBooleans are the spellings a spreadsheet uses for yes and no.
//
//nolint:goconst // a cell's spelling; the constant goconst offers names a query argument's
var spreadsheetBooleans = map[string]bool{"true": true, "yes": true, "1": true, "false": false, "no": false, "0": false}

// booleanValue reads a cell spelled as one of spreadsheetBooleans.
func booleanValue(text string) (bool, string) {
	if value, ok := spreadsheetBooleans[strings.ToLower(text)]; ok {
		return value, ""
	}
	return false, fmt.Sprintf("%q is not yes or no", text)
}

func optionNamed(options []string, text string) (string, bool) {
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option), text) {
			return option, true
		}
	}
	return "", false
}

func notAllowedMessage(col fieldcatalog.Column, text string, optionsReadable bool) string {
	if !optionsReadable {
		return fmt.Sprintf("%q is not one of this field's allowed values", text)
	}
	return fmt.Sprintf("%q is not one of the allowed values: %s", text, strings.Join(col.Options, ", "))
}

// unboundMessage says why SQLValue refused a value of this type.
func unboundMessage(col fieldcatalog.Column, text string) string {
	switch col.Type {
	case fieldcatalog.TypeNumber:
		return fmt.Sprintf("%q is not a number", text)
	case fieldcatalog.TypeDate:
		return fmt.Sprintf("%q is not a date; write it as YYYY-MM-DD", text)
	}
	return fmt.Sprintf("%q cannot be written to a %s field", text, col.Type)
}

// wireText renders a checked value the way textOf renders the stored JSON, so
// an unedited cell compares equal to what it wrote.
//
//craft:ignore naked-any the value is one checkCell answered, in SQLValue's input shape
func wireText(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("import: rendering a checked value: %w", err)
	}
	return textOf(encoded), nil
}

// headerFor is the file's column mapped onto target, so a problem names the
// header the user typed rather than the field's key.
func (w *csvWriters) headerFor(target string) string {
	return columnFor(w.columns, target)
}

// checkedCells is checkRow for one of this part's rows, with a problem
// rendered as the report's invalid_value issue on the file's column.
func (w *csvWriters) checkedCells(row migration.Row) (map[string]any, *migration.SkippedRow) {
	typed, problem := checkRow(w.object, textFields(row.Fields), w.cols.catalog)
	if problem == nil {
		return typed, nil
	}
	return nil, &migration.SkippedRow{
		ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(problem.Target),
		Kind: migration.ProblemInvalidValue, Value: problem.Value, Reason: problem.Message,
	}
}

// unwritableReason is the core-field half of checkRow: a value the store
// refuses before or inside the write transaction.
//
// The size_band vocabulary is read off the generated contract, not
// hand-copied, so a band added to crm.yaml leaves no second list behind.
// Held by: TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt (backend/gates/contractvocabulary_test.go)
func unwritableReason(object string, fields map[string]string) *cellProblem {
	if reason := authorUnwritableReason(fields); reason != "" {
		return &cellProblem{Target: csvTargetAuthor, Value: fields[csvTargetAuthor], Message: reason}
	}
	if object == migration.ObjectContact {
		// parseContactContacts refuses a malformed address before the write.
		email := strings.TrimSpace(fields[fieldEmail])
		if email == "" {
			return nil
		}
		if _, err := values.ParseEmail(email); err != nil {
			return &cellProblem{
				Target: fieldEmail, Value: fields[fieldEmail],
				Message: fmt.Sprintf("%q is not an email address that can be written", fields[fieldEmail]),
			}
		}
		return nil
	}
	if object != migration.ObjectCompany {
		return nil
	}
	// parseCompanyDomains refuses a domain its parser cannot read before the write.
	if domain := strings.TrimSpace(fields[fieldDomain]); domain != "" {
		if _, err := values.ParseDomain(domain); err != nil {
			return &cellProblem{
				Target: fieldDomain, Value: fields[fieldDomain],
				Message: fmt.Sprintf("%q is not a domain that can be written", domain),
			}
		}
	}
	band := strings.TrimSpace(fields[fieldSizeBand])
	if band == "" || importableSizeBands[band] {
		return nil
	}
	return &cellProblem{
		Target: fieldSizeBand, Value: fields[fieldSizeBand],
		Message: fmt.Sprintf("size_band %q is not one of %s", fields[fieldSizeBand], strings.Join(sizeBandVocabulary(), ", ")),
	}
}

// importableSizeBands is the closed size_band vocabulary, off the contract.
var importableSizeBands = map[string]bool{
	string(crmcontracts.CompanySizeBandN110):      true,
	string(crmcontracts.CompanySizeBandN1150):     true,
	string(crmcontracts.CompanySizeBandN51200):    true,
	string(crmcontracts.CompanySizeBandN201500):   true,
	string(crmcontracts.CompanySizeBandN5011000):  true,
	string(crmcontracts.CompanySizeBandN10015000): true,
	string(crmcontracts.CompanySizeBandN5000):     true,
}

// sizeBandVocabulary renders the bands in ascending order, so a refusal reads
// the same on every call.
func sizeBandVocabulary() []string {
	out := make([]string, 0, len(importableSizeBands))
	for band := range importableSizeBands {
		out = append(out, band)
	}
	sort.Strings(out)
	return out
}
