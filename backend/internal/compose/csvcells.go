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
	"strconv"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// cellProblem is one cell the store would refuse. Target is the mapping
// target; the part writer turns it into the file's own header.
type cellProblem struct {
	Target, Value, Message string
	// Why is Message as a code, so the report can say it in another language.
	Why migration.Reason
}

// problemOf is one cell the store would refuse, worded by the reason.
func problemOf(target, value string, why migration.Reason) *cellProblem {
	return &cellProblem{Target: target, Value: value, Message: why.English(), Why: why}
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
		if !datasource.IsCustomFieldKey(target) || strings.TrimSpace(raw) == "" {
			continue
		}
		col, active := cat.byName[target]
		if !active {
			// Retired after the run was staged: the store has no column to bind it to.
			return nil, problemOf(target, raw, migration.Why(migration.ReasonFieldInactive, "value", strings.TrimSpace(raw)))
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
			return nil, problemOf(target, fields[target], parseReason(text, err))
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

func parseReason(text string, err error) migration.Reason {
	parseErr, ok := errors.AsType[*values.ParseError](err)
	if !ok {
		return migration.Why(migration.ReasonValueUnreadable, "value", text)
	}
	if parseErr.Code == "phone_needs_country_code" {
		return migration.Why(migration.ReasonPhoneNeedsCountry, "value", text)
	}
	return migration.Why(migration.ReasonValueRefused, "value", text, "detail", parseErr.Message)
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
	var message migration.Reason
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
	if message.Code == "" {
		if _, binds := storekit.SQLValue(col, wire); !binds {
			message = unboundReason(col, text)
		}
	}
	if message.Code != "" {
		return nil, problemOf(target, raw, message)
	}
	return wire, nil
}

// picklistValue is the catalog's spelling of the option the text names.
func picklistValue(col fieldcatalog.Column, text string, optionsReadable bool) (string, migration.Reason) {
	if option, found := optionNamed(col.Options, text); found {
		return option, migration.Reason{}
	}
	return "", notAllowedReason(col, text, optionsReadable)
}

// multiselectValue splits on semicolons and names each option in the catalog's
// spelling. A comma with no semicolon is the usual mistake, so it gets its own
// sentence once the whole text is not itself an option.
func multiselectValue(col fieldcatalog.Column, text string, optionsReadable bool) ([]string, migration.Reason) {
	var chosen []string
	for part := range strings.SplitSeq(text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		option, found := optionNamed(col.Options, part)
		if !found {
			if strings.Contains(text, ",") && !strings.Contains(text, ";") {
				return nil, migration.Why(migration.ReasonOptionsNeedSemicolons, "value", text)
			}
			return nil, notAllowedReason(col, part, optionsReadable)
		}
		chosen = append(chosen, option)
	}
	return chosen, migration.Reason{}
}

// currencyValue is the amount in the column's minor units. A column with no
// currency has no unit to count, so its values cannot be read at all.
func currencyValue(col fieldcatalog.Column, text string) (float64, migration.Reason) {
	if col.Currency == "" {
		return 0, migration.Why(migration.ReasonCurrencyUnset, "value", text)
	}
	minor, ok := values.MinorUnits(text, col.Currency)
	if !ok {
		return 0, migration.Why(migration.ReasonNotAnAmount, "value", text, "currency", col.Currency)
	}
	return float64(minor), migration.Reason{}
}

// spreadsheetBooleans are the spellings a spreadsheet uses for yes and no.
//
//nolint:goconst // a cell's spelling; the constant goconst offers names a query argument's
var spreadsheetBooleans = map[string]bool{"true": true, "yes": true, "1": true, "false": false, "no": false, "0": false}

// booleanValue reads a cell spelled as one of spreadsheetBooleans.
func booleanValue(text string) (bool, migration.Reason) {
	if value, ok := spreadsheetBooleans[strings.ToLower(text)]; ok {
		return value, migration.Reason{}
	}
	return false, migration.Why(migration.ReasonNotYesNo, "value", text)
}

func optionNamed(options []string, text string) (string, bool) {
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option), text) {
			return option, true
		}
	}
	return "", false
}

func notAllowedReason(col fieldcatalog.Column, text string, optionsReadable bool) migration.Reason {
	if !optionsReadable {
		return migration.Why(migration.ReasonOptionNotAllowed, "value", text)
	}
	return migration.Why(migration.ReasonOptionNotInList, "value", text, "options", strings.Join(col.Options, ", "))
}

// unboundReason says why SQLValue refused a value of this type.
func unboundReason(col fieldcatalog.Column, text string) migration.Reason {
	switch col.Type {
	case fieldcatalog.TypeNumber:
		return migration.Why(migration.ReasonNotANumber, "value", text)
	case fieldcatalog.TypeDate:
		return migration.Why(migration.ReasonNotADate, "value", text)
	}
	return migration.Why(migration.ReasonCannotWriteType, "value", text, "type", string(col.Type))
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
	fields := textFields(row.Fields)
	typed, problem := checkRow(w.object, fields, w.cols.catalog)
	if problem == nil && w.object == migration.ObjectDeal {
		typed, problem = w.dealStages.checkCells(fields, typed)
	}
	if problem == nil && w.object == migration.ObjectLead {
		typed, problem = checkLeadStatus(fields, typed)
	}
	if problem == nil && w.object == migration.ObjectActivity {
		typed, problem = checkActivityCells(fields, typed, w.nowUTC())
	}
	if problem == nil {
		return typed, nil
	}
	return nil, &migration.SkippedRow{
		ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(problem.Target),
		Kind: migration.ProblemInvalidValue, Value: problem.Value,
		Reason: problem.Message, Code: problem.Why.Code, Params: problem.Why.Params,
	}
}

// unwritableReason is the core-field half of checkRow: a value the store
// refuses before or inside the write transaction.
//
// The size_band vocabulary is read off the generated contract, not
// hand-copied, so a band added to crm.yaml leaves no second list behind.
// Held by: TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt (backend/gates/contractvocabulary_test.go)
func unwritableReason(object string, fields map[string]string) *cellProblem {
	if reason := authorUnwritableReason(fields); reason.Code != "" {
		return problemOf(csvTargetAuthor, fields[csvTargetAuthor], reason)
	}
	if problem := tagCellProblem(fields[csvTargetTags]); problem != nil {
		return problem
	}
	if object == migration.ObjectContact || object == migration.ObjectCompany {
		if problem := countryCellProblem(fields); problem != nil {
			return problem
		}
	}
	if object == migration.ObjectContact || object == migration.ObjectLead {
		// Both stores parse the address before the write and refuse a malformed one.
		email := strings.TrimSpace(fields[fieldEmail])
		if email == "" {
			return nil
		}
		if _, err := values.ParseEmail(email); err != nil {
			return problemOf(fieldEmail, fields[fieldEmail], migration.Why(migration.ReasonNotAnEmail, "value", fields[fieldEmail]))
		}
		// The contacts store refuses a sending system's address; a lead may hold one.
		if object == migration.ObjectContact && mailidentity.HasMachineLocalpart(email) {
			return problemOf(fieldEmail, fields[fieldEmail], migration.Why(migration.ReasonEmailNamesNoContact, "value", email))
		}
		return nil
	}
	if object != migration.ObjectCompany {
		return nil
	}
	// parseCompanyDomains refuses a domain its parser cannot read before the write.
	if domain := strings.TrimSpace(fields[fieldDomain]); domain != "" {
		if _, err := values.ParseDomain(domain); err != nil {
			return problemOf(fieldDomain, fields[fieldDomain], migration.Why(migration.ReasonNotADomain, "value", domain))
		}
	}
	if length := utf8.RuneCountInString(fields[descriptionField]); length > contacts.CompanyDescriptionMax {
		return problemOf(descriptionField, fields[descriptionField], migration.Why(migration.ReasonDescriptionTooLong,
			"length", strconv.Itoa(length), "max", strconv.Itoa(contacts.CompanyDescriptionMax)))
	}
	band := strings.TrimSpace(fields[fieldSizeBand])
	if band == "" || importableSizeBands[band] {
		return nil
	}
	return problemOf(fieldSizeBand, fields[fieldSizeBand], migration.Why(migration.ReasonSizeBandInvalid,
		"value", fields[fieldSizeBand], "allowed", strings.Join(sizeBandVocabulary(), ", ")))
}

// countryCellProblem refuses an address country the stores would refuse, by the
// store's own rule, so the preview and the commit agree on the cell.
func countryCellProblem(fields map[string]string) *cellProblem {
	raw := fields[fieldAddressCountry]
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if _, err := contacts.NormalizeCountryCode(raw); err != nil {
		return problemOf(fieldAddressCountry, raw, migration.Why(migration.ReasonValueRefused, "value", raw, "detail", err.Error()))
	}
	return nil
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
