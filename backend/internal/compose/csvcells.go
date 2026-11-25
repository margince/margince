// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every cell of a row is checked before anything is written. The dry run and
// the commit both ask here, so a preview cannot promise a row the store refuses.

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/compose/importtextbounds"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/owndomain"
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

// cellProblems collects the cells of one row the store would refuse, in the
// order they were checked.
type cellProblems []*cellProblem

// add keeps a check's problem and passes over a check that found none.
func (p *cellProblems) add(problem *cellProblem) {
	if problem != nil {
		*p = append(*p, problem)
	}
}

// checkRow checks the core cells, then each custom-field cell in target order.
// It answers the filled phone, LinkedIn and custom-field cells in the shape the
// store holds them. A row the store would refuse answers each refused cell.
func checkRow(object string, fields map[string]string, cat importCatalog) (map[string]any, []*cellProblem) {
	problems := unwritableCells(object, fields)
	typed, unparsed := checkParsedCells(object, fields)
	problems = append(problems, unparsed...)
	for _, target := range slices.Sorted(maps.Keys(fields)) {
		raw := fields[target]
		if !datasource.IsCustomFieldKey(target) || strings.TrimSpace(raw) == "" {
			continue
		}
		col, active := cat.byName[target]
		if !active {
			// Retired after the run was staged: the store has no column to bind it to.
			problems = append(problems, problemOf(target, raw, migration.Why(migration.ReasonFieldInactive, "value", strings.TrimSpace(raw))))
			continue
		}
		value, problem := checkCell(target, col, raw, cat.optionsReadable)
		if problem != nil {
			problems = append(problems, problem)
			continue
		}
		if typed == nil {
			typed = map[string]any{}
		}
		typed[target] = value
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return typed, nil
}

// checkParsedCells reads the phone and LinkedIn cells through the parsers the
// store writes with, so the row carries the spelling the store will hold.
func checkParsedCells(object string, fields map[string]string) (map[string]any, []*cellProblem) {
	var typed map[string]any
	var problems []*cellProblem
	for _, target := range []string{fieldPhone, fieldLinkedIn} {
		text := strings.TrimSpace(fields[target])
		if text == "" {
			continue
		}
		stored, err := storedSpelling(object, target, text)
		if err != nil {
			problems = append(problems, problemOf(target, fields[target], parseReason(target, text, err)))
			continue
		}
		if typed == nil {
			typed = map[string]any{}
		}
		typed[target] = stored
	}
	return typed, problems
}

// storedSpelling is the parser each object's store runs on a phone or a
// LinkedIn URL. A company takes a company page; a contact and a lead take a
// profile.
func storedSpelling(object, target, text string) (string, error) {
	switch {
	case target == fieldPhone:
		phone, err := values.ParsePhone(text)
		return phone.String(), err
	case object == migration.ObjectCompany:
		return contacts.NormalizeCompanyLinkedInURL(text)
	default:
		return contacts.NormalizeContactLinkedInURL(text)
	}
}

func parseReason(target, text string, err error) migration.Reason {
	parseErr, ok := errors.AsType[*values.ParseError](err)
	if !ok {
		return migration.Why(migration.ReasonValueUnreadable, "value", text)
	}
	if target == fieldPhone {
		return phoneReason(text, parseErr)
	}
	return migration.Why(migration.ReasonValueRefused, "value", text, "detail", parseErr.Message)
}

// phoneReason asks for the country code only of a cell that holds a number.
// Text with more in it than digits, a plus and separators is no phone number.
func phoneReason(text string, parseErr *values.ParseError) migration.Reason {
	const digits = "0123456789"
	numeric := strings.ContainsAny(text, digits) &&
		strings.Trim(text, digits+"+"+strings.Join(values.PhoneSeparators(), "")) == ""
	switch {
	case !numeric:
		return migration.Why(migration.ReasonNotAPhone, "value", text)
	case parseErr.Code == "phone_needs_country_code":
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
		return 0, amountReason(text, col.Currency)
	}
	return float64(minor), migration.Reason{}
}

// amountReason says what is wrong with text as an amount in currency: its
// sign, its decimal places, or its form. The example is one the currency takes.
func amountReason(text, currency string) migration.Reason {
	digits := values.MinorUnitDigits(currency)
	unsigned, negative := strings.CutPrefix(text, "-")
	scale := values.MinorUnitScale(currency)
	example := values.MajorUnits(12000*scale+scale/2, currency)
	switch {
	case negative && values.PlainDecimal(unsigned, len(unsigned), len(unsigned)):
		return migration.Why(migration.ReasonAmountNegative, "value", text)
	case values.PlainDecimal(text, len(text), len(text)) && !values.PlainDecimal(text, len(text), digits):
		return migration.Why(migration.ReasonAmountTooManyDecimals, "value", text, "currency", currency, "example", example)
	}
	return migration.Why(migration.ReasonNotAnAmount, "value", text, "currency", currency, "example", example)
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

// checkedCells is checkRow for one of this part's rows, then the cells only
// its object has. Every problem is rendered as the report's invalid_value
// issue on the file's column, so one upload shows all a row needs corrected.
func (w *csvWriters) checkedCells(row migration.Row) (map[string]any, []migration.SkippedRow) {
	fields := textFields(row.Fields)
	typed, problems := checkRow(w.object, fields, w.cols.catalog)
	var own []*cellProblem
	switch w.object {
	case migration.ObjectDeal:
		typed, own = w.dealStages.checkCells(fields, typed)
	case migration.ObjectLead:
		var problem *cellProblem
		typed, problem = checkLeadStatus(fields, typed)
		if problem != nil {
			own = append(own, problem)
		}
	case migration.ObjectActivity:
		typed, own = checkActivityCells(fields, typed, w.cellClock())
	}
	problems = append(problems, own...)
	if len(problems) == 0 {
		return typed, nil
	}
	invalid := make([]migration.SkippedRow, 0, len(problems))
	for _, problem := range problems {
		invalid = append(invalid, migration.SkippedRow{
			ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(problem.Target),
			Kind: migration.ProblemInvalidValue, Value: problem.Value,
			Reason: problem.Message, Code: problem.Why.Code, Params: problem.Why.Params,
		})
	}
	return nil, invalid
}

// unwritableCells is the core-field half of checkRow: each value the store
// refuses before or inside the write transaction.
func unwritableCells(object string, fields map[string]string) []*cellProblem {
	var problems cellProblems
	if reason := authorUnwritableReason(fields); reason.Code != "" {
		problems.add(problemOf(csvTargetAuthor, fields[csvTargetAuthor], reason))
	}
	problems.add(tagCellProblem(fields[csvTargetTags]))
	if object == migration.ObjectContact || object == migration.ObjectCompany {
		problems.add(countryCellProblem(fields))
	}
	problems.add(emailCellProblem(object, fields))
	if target, value, why, refused := importtextbounds.Refusal(object, fields); refused {
		problems.add(problemOf(target, value, why))
	}
	if object == migration.ObjectCompany {
		problems.add(domainCellProblem(fields))
		problems.add(sizeBandCellProblem(fields))
	}
	return problems
}

// domainCellProblem refuses a domain the companies store's parser cannot read
// before the write.
func domainCellProblem(fields map[string]string) *cellProblem {
	domain := strings.TrimSpace(fields[fieldDomain])
	if domain == "" {
		return nil
	}
	if _, err := owndomain.ParseDomain(domain); err != nil {
		return problemOf(fieldDomain, fields[fieldDomain], migration.Why(migration.ReasonNotADomain, "value", domain))
	}
	return nil
}

// sizeBandCellProblem refuses a size band outside the contract's vocabulary.
//
// The vocabulary is read off the generated contract, not hand-copied, so a
// band added to crm.yaml leaves no second list behind.
// Held by: TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt (backend/gates/contractvocabulary_test.go)
func sizeBandCellProblem(fields map[string]string) *cellProblem {
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

// sizeBandVocabulary renders the bands from the smallest company to the
// largest, by the head count each band starts at.
func sizeBandVocabulary() []string {
	bands := slices.Collect(maps.Keys(importableSizeBands))
	slices.SortFunc(bands, func(a, b string) int {
		return cmp.Or(cmp.Compare(firstHeadCount(a), firstHeadCount(b)), cmp.Compare(a, b))
	})
	return bands
}

// firstHeadCount is the number a band opens with: 11 for "11-50", 5000 for "5000+".
func firstHeadCount(band string) int {
	end := strings.IndexFunc(band, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		end = len(band)
	}
	count, err := strconv.Atoi(band[:end])
	if err != nil {
		// A band that opens with no number sorts first, where a reader sees it.
		return -1
	}
	return count
}

// emailCellProblem refuses an address the store would refuse. Every store
// parses it before the write, so a malformed one is a problem of the test run.
// The contacts store also refuses a sending system's address; a lead or a
// company may hold one.
func emailCellProblem(object string, fields map[string]string) *cellProblem {
	email := strings.TrimSpace(fields[fieldEmail])
	if email == "" {
		return nil
	}
	if _, err := values.ParseEmail(email); err != nil {
		return problemOf(fieldEmail, fields[fieldEmail], migration.Why(migration.ReasonNotAnEmail, "value", fields[fieldEmail]))
	}
	if object == migration.ObjectContact && mailidentity.HasMachineLocalpart(email) {
		return problemOf(fieldEmail, fields[fieldEmail], migration.Why(migration.ReasonEmailNamesNoContact, "value", email))
	}
	return nil
}
