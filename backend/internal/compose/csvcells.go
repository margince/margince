// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every cell of a row is checked before anything is written. The dry run and
// the commit both ask here, so a preview cannot promise a row the store refuses.

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/compose/importcells"
	"github.com/margince/margince/backend/internal/compose/importtextbounds"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
		value, why := importcells.FieldValue(col, raw, cat.optionsReadable)
		if why.Code != "" {
			problems = append(problems, problemOf(target, raw, why))
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
		stored, why := storedSpelling(object, target, text)
		if why.Code != "" {
			problems = append(problems, problemOf(target, fields[target], why))
			continue
		}
		if typed == nil {
			typed = map[string]any{}
		}
		typed[target] = stored
	}
	return typed, problems
}

// storedSpelling reads a phone or a LinkedIn cell as the object's store holds it.
func storedSpelling(object, target, text string) (string, migration.Reason) {
	if target == fieldPhone {
		return importcells.Phone(text)
	}
	return importcells.LinkedIn(object, text)
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
func sizeBandCellProblem(fields map[string]string) *cellProblem {
	band := strings.TrimSpace(fields[fieldSizeBand])
	if band == "" || importcells.IsSizeBand(band) {
		return nil
	}
	return problemOf(fieldSizeBand, fields[fieldSizeBand], migration.Why(migration.ReasonSizeBandInvalid,
		"value", fields[fieldSizeBand], "allowed", strings.Join(importcells.SizeBandVocabulary(), ", ")))
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
