// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A file whose only address column is mapped to line 1 usually holds the whole
// address in it. The cell is split with the splitter the company writers use.
// The split happens before any reader sees the row, so the dry run, the review
// and the commit compare and write the same parts.

import (
	"strings"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
)

const addressLine1Target = "address.line1"

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

// shapeAddressCells sets the line 1 split on a part's source when its mapping
// asks for one.
func shapeAddressCells(source *migration.CSVSource, object string, mapping map[string]string) {
	if splitsAddressLine(object, mapping) {
		source.ShapeRows(splitAddressCell)
	}
}

// splitAddressCell replaces line 1 with its parts. A cell the splitter refuses
// stays whole in line 1, and the row's warning says so.
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
		addressLine1Target: address.Line1, "address.line2": address.Line2,
		"address.region": address.Region, "address.postal_code": address.PostalCode,
		"address.city": address.City, fieldAddressCountry: address.Country,
	} {
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
	if fields[addressLine1Target] == "" || fields["address.city"] != "" {
		return nil
	}
	warning := migration.SkippedRow{
		ExternalID: row.ExternalID, Line: row.Line, Column: w.headerFor(addressLine1Target),
		Kind: migration.ProblemInvalidValue, Value: fields[addressLine1Target],
	}.Because(addressNotSplitWhy)
	return &warning
}
