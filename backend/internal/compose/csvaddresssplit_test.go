// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
)

func TestOnlyALineOneMappedAloneIsSplit(t *testing.T) {
	cases := []struct {
		object  string
		mapping map[string]string
		want    bool
	}{
		{migration.ObjectCompany, map[string]string{"Name": "display_name", "Address": "address.line1"}, true},
		{migration.ObjectContact, map[string]string{"Address": "address.line1"}, true},
		{migration.ObjectCompany, map[string]string{"Street": "address.line1", "City": "address.city"}, false},
		{migration.ObjectCompany, map[string]string{"Street": "address.line1", "Country": "address.country"}, false},
		{migration.ObjectCompany, map[string]string{"City": "address.city"}, false},
		{migration.ObjectLead, map[string]string{"Address": "address.line1"}, false},
	}
	for _, c := range cases {
		if got := splitsAddressLine(c.object, c.mapping); got != c.want {
			t.Errorf("%s %v: split = %v, want %v", c.object, c.mapping, got, c.want)
		}
	}
}

func TestASplitCellBecomesTheAddressParts(t *testing.T) {
	fields := map[string]any{"display_name": "Splitwerk GmbH", "address.line1": "Example Street 4, Tower B, 12345 Example City, Germany"}
	splitAddressCell(fields)
	want := map[string]any{
		"display_name": "Splitwerk GmbH", "address.line1": "Example Street 4", "address.line2": "Tower B",
		"address.postal_code": "12345", "address.city": "Example City", "address.country": "DE",
	}
	if !maps.Equal(fields, want) {
		t.Errorf("fields = %v, want %v", fields, want)
	}

	refused := map[string]any{"address.line1": "Somewhere near the river"}
	splitAddressCell(refused)
	if !maps.Equal(refused, map[string]any{"address.line1": "Somewhere near the river"}) {
		t.Errorf("a refused cell changed: %v", refused)
	}
}
