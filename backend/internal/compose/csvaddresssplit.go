// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A part's cells are shaped before any reader sees the row. The dry run, the
// review and the commit then compare and write the same text. A name folds onto
// one line. A file whose only address column is mapped to line 1 usually holds
// the whole address in it, which is split with the company writers' splitter.

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// shapeCells sets the rewrites every reader of a part's rows sees, so the
// test run and the commit check and write one spelling. Names fold first, then
// the address cells change as the mapping asks.
func shapeCells(source *migration.CSVSource, object string, mapping map[string]string) {
	address := addressShape(object, mapping)
	source.ShapeRows(func(fields map[string]any) {
		foldNameCells(fields)
		if address != nil {
			address(fields)
		}
	})
}

// importNameTargets are the cells that name a record, for every type a file
// may hold. A line break or a run of spaces in one is a slip in the file.
var importNameTargets = []string{fieldFullName, "first_name", "last_name", fieldDisplayName, fieldLegalName, dealTargetName}

// foldNameCells folds each run of white space in a name cell to one space.
func foldNameCells(fields map[string]any) {
	for _, target := range importNameTargets {
		if name, ok := fields[target].(string); ok {
			fields[target] = values.CollapseSpace(name)
		}
	}
}

const (
	addressLine1Target      = "address.line1"
	addressLine2Target      = "address.line2"
	addressCityTarget       = "address.city"
	addressRegionTarget     = "address.region"
	addressPostalCodeTarget = "address.postal_code"
)

// splitsAddressLine reports whether a part's mapping names line 1 and no other
// address part. A file that maps the city keeps its line 1 as written.
func splitsAddressLine(object string, mapping map[string]string) bool {
	if object != migration.ObjectCompany && object != migration.ObjectContact {
		return false
	}
	line1 := false
	for _, target := range mapping {
		switch {
		case target == addressLine1Target:
			line1 = true
		case strings.HasPrefix(target, "address."):
			return false
		}
	}
	return line1
}

// addressShape is the cell rewrite a part's mapping asks for: the line 1
// split, and a country name read as its code. It is nil when it asks for none.
func addressShape(object string, mapping map[string]string) migration.RowShape {
	split := splitsAddressLine(object, mapping)
	country := object != migration.ObjectLead && slices.Contains(slices.Collect(maps.Values(mapping)), fieldAddressCountry)
	if !split && !country {
		return nil
	}
	return func(fields map[string]any) {
		if split {
			splitAddressCell(fields)
		}
		if country {
			resolveCountryCell(fields)
		}
	}
}

// resolveCountryCell turns a country written as a name into its code. A value
// it cannot place stays as written, so the cell check refuses it with the row named.
func resolveCountryCell(fields map[string]any) {
	if raw, ok := fields[fieldAddressCountry].(string); ok {
		if code, found := contacts.ResolveCountry(raw); found {
			fields[fieldAddressCountry] = code
		}
	}
}

// splitAddressCell replaces line 1 with its parts. A part the split leaves
// unset is an empty cell, which clears what the record held: the split stands
// for the whole address. A cell the splitter refuses stays whole in line 1, and
// the row's warning says so.
func splitAddressCell(fields map[string]any) {
	raw, ok := fields[addressLine1Target].(string)
	if !ok {
		return
	}
	address, split := contacts.SplitAddressText(raw)
	if !split {
		return
	}
	for target, part := range map[string]*string{
		addressLine1Target: address.Line1, addressLine2Target: address.Line2,
		addressRegionTarget: address.Region, addressPostalCodeTarget: address.PostalCode,
		addressCityTarget: address.City, fieldAddressCountry: address.Country,
	} {
		fields[target] = ""
		if part != nil {
			fields[target] = *part
		}
	}
}

// unsplitAddressWarning is the warning a row lands with when its line 1 was to
// be split and the splitter refused it. A split always yields a city.
func (w *csvWriters) unsplitAddressWarning(row migration.Row) *migration.SkippedRow {
	if !splitsAddressLine(w.object, w.columns) {
		return nil
	}
	fields := textFields(row.Fields)
	if fields[addressLine1Target] == "" || fields[addressCityTarget] != "" {
		return nil
	}
	warning := migration.SkippedRow{
		ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(addressLine1Target),
		Kind: migration.ProblemInvalidValue, Value: fields[addressLine1Target],
	}.Because(addressNotSplitWhy)
	return &warning
}

func addressMergedOnto(current []byte, mapped *crmcontracts.Address, cells map[string]string) (*crmcontracts.Address, bool, error) {
	if mapped == nil {
		return nil, false, nil
	}

	var record struct {
		Address *crmcontracts.Address `json:"address"`
	}
	if err := json.Unmarshal(current, &record); err != nil {
		return nil, false, fmt.Errorf("import: reading the stored address: %w", err)
	}
	if record.Address == nil {
		return mapped, true, nil
	}
	merged := *record.Address
	clearEmptiedParts(&merged, cells)
	if mapped.Line1 != nil {
		merged.Line1 = mapped.Line1
	}
	if mapped.Line2 != nil {
		merged.Line2 = mapped.Line2
	}
	if mapped.City != nil {
		merged.City = mapped.City
	}
	if mapped.Region != nil {
		merged.Region = mapped.Region
	}
	if mapped.PostalCode != nil {
		merged.PostalCode = mapped.PostalCode
	}
	if mapped.Country != nil {
		merged.Country = mapped.Country
	}
	return &merged, true, nil
}

// addressFrom collects the mapped `address.*` columns into the one nested
// value both the create and the patch input take. It is absent when the row
// names no address cell. The update path would read an empty address as an
// instruction to clear one.
func addressFrom(fields map[string]string) *crmcontracts.Address {
	given := false
	for _, target := range recordAddressTargets {
		_, present := fields[target]
		given = given || present
	}
	addr := crmcontracts.Address{
		Line1:      importString(fields, addressLine1Target),
		Line2:      importString(fields, addressLine2Target),
		City:       importString(fields, addressCityTarget),
		Region:     importString(fields, addressRegionTarget),
		PostalCode: importString(fields, addressPostalCodeTarget),
		Country:    importString(fields, fieldAddressCountry),
	}
	if !given {
		return nil
	}
	return &addr
}

// clearEmptiedParts drops each stored part whose cell is present and empty. Only
// a split line 1 writes such a cell, so the split replaces the whole address.
func clearEmptiedParts(stored *crmcontracts.Address, cells map[string]string) {
	for target, part := range map[string]**string{
		addressLine1Target: &stored.Line1, addressLine2Target: &stored.Line2, addressCityTarget: &stored.City,
		addressRegionTarget: &stored.Region, addressPostalCodeTarget: &stored.PostalCode, fieldAddressCountry: &stored.Country,
	} {
		if value, present := cells[target]; present && value == "" {
			*part = nil
		}
	}
}
