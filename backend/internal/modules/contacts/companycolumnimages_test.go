// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"maps"
	"testing"
)

// An Undo writes the address back as one object. An image without the parts
// that did not move would clear them.
func TestAMovedAddressPartKeepsTheWholeAddressInBothImages(t *testing.T) {
	before := emptyColdStartColumnImages()
	before[columnAddressLine1], before[columnAddressCity], before[columnAddressCountry] = "Old Street 1", "Berlin", "DE"
	before[columnIndustry] = "Logistics"
	after := maps.Clone(before)
	after[columnAddressLine1] = "New Street 2"

	was, now := changedColdStartColumns(before, after)
	for _, column := range companyAddressColumns {
		if _, kept := was[column]; !kept {
			t.Errorf("the before image dropped %s, which the Undo would then clear", column)
		}
		if now[column] != after[column] {
			t.Errorf("the after image holds %s = %v, want %v", column, now[column], after[column])
		}
	}
	if was[columnAddressCity] != "Berlin" || was[columnAddressLine1] != "Old Street 1" {
		t.Errorf("the before image is %v, want the address as it stood", was)
	}
	if _, kept := was[columnIndustry]; kept {
		t.Error("a column that did not move is in the image")
	}
}

func TestAnUnmovedAddressStaysOutOfTheImages(t *testing.T) {
	before := emptyColdStartColumnImages()
	after := emptyColdStartColumnImages()
	after[columnIndustry] = "Logistics"
	was, now := changedColdStartColumns(before, after)
	if len(was) != 1 || len(now) != 1 {
		t.Errorf("images = %v / %v, want only the industry", was, now)
	}
}

func TestAReceiptIsKeptOnlyForARowBehindAMovedColumn(t *testing.T) {
	row := func(version int64) profileFieldRow { return profileFieldRow{Version: version, Content: []byte(`{}`)} }
	before := map[string]profileFieldRow{fieldIndustry: row(1)}
	after := map[string]profileFieldRow{fieldIndustry: row(2), fieldRegisteredAddress: row(1), fieldLegalName: row(1)}
	receipts := profileFieldReceipts(before, after, map[string]any{columnAddressCity: "Berlin", columnIndustry: "Logistics"})
	if len(receipts) != 2 {
		t.Fatalf("receipts = %v, want industry and registered_address", receipts)
	}
	if receipts[fieldRegisteredAddress].Before != nil || receipts[fieldRegisteredAddress].After == nil {
		t.Errorf("a row the apply created reads %+v, want no before and an after", receipts[fieldRegisteredAddress])
	}
	if receipts[fieldIndustry].Before == nil || receipts[fieldIndustry].Before.Version != 1 {
		t.Errorf("a row the apply replaced reads %+v, want the earlier version as before", receipts[fieldIndustry])
	}
}

// Both catalogs that couple a profile field to a column must name the same
// column, or the receipt map keeps whichever one it read last.
func TestTheCatalogsCouplingAFieldToAColumnAgree(t *testing.T) {
	for _, spec := range companyFields {
		if spec.column == "" {
			continue
		}
		if column, backed := columnBackedColdStartFields[spec.name]; backed && column != spec.column {
			t.Errorf("%s writes %s from the form and %s from a read-back", spec.name, spec.column, column)
		}
	}
	for field := range columnBackedColdStartFields {
		if _, receipted := profileFieldColumns[field]; !receipted {
			t.Errorf("%s has a column but no receipt mapping", field)
		}
	}
}
