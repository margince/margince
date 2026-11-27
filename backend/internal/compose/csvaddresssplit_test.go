// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"maps"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
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
		"address.postal_code": "12345", "address.city": "Example City", "address.country": "DE", "address.region": "",
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

func TestASplitLineOneReplacesTheWholeStoredAddress(t *testing.T) {
	cells := map[string]any{"address.line1": "Neue Straße 1, 10115 Berlin, Germany"}
	splitAddressCell(cells)
	changed := textFields(cells)
	stored := []byte(`{"address":{"line1":"Old Road 2","line2":"Old Suite","region":"CA","postal_code":"94043","city":"Mountain View","country":"US"}}`)
	merged, given, err := addressMergedOnto(stored, addressFrom(changed), changed)
	if err != nil || !given {
		t.Fatalf("merged = %+v (given %v, err %v)", merged, given, err)
	}
	got := [6]string{}
	for i, part := range []*string{merged.Line1, merged.Line2, merged.Region, merged.PostalCode, merged.City, merged.Country} {
		got[i] = "<nil>"
		if part != nil {
			got[i] = *part
		}
	}
	if want := [6]string{"Neue Straße 1", "<nil>", "<nil>", "10115", "Berlin", "DE"}; got != want {
		t.Errorf("address = %q, want %q", got, want)
	}
}

// A quoted name over two lines, or with a run of spaces, is read as one line.
// The source folds it, so every reader of the row sees that one spelling.
// Other text keeps its line breaks.
func TestAnImportedNameIsReadOnOneLine(t *testing.T) {
	ctx := context.Background()
	file := "email,full_name,title\nada@example.com,\"Ada\n  Lovelace \",\"Head of\nResearch\"\n"
	blobs := blobstore.NewMemory()
	if err := blobs.Put(ctx, "contacts.csv", strings.NewReader(file), int64(len(file)), "text/csv"); err != nil {
		t.Fatal(err)
	}
	mapping := migration.RunMapping{
		Object: migration.ObjectContact, SourceKey: "email",
		Fields: map[string]string{"email": fieldEmail, "full_name": fieldFullName, "title": fieldTitle},
	}
	source, _, err := bundleSourceFor(blobs, mapping, "contacts.csv", int64(len(file)))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := source.Rows(ctx, migration.ObjectContact, 0, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v, %v; want the one row", rows, err)
	}
	if got := rows[0].Fields[fieldFullName]; got != "Ada Lovelace" {
		t.Errorf("full_name = %q, want the name on one line", got)
	}
	if got := rows[0].Fields[fieldTitle]; got != "Head of\nResearch" {
		t.Errorf("title = %q, want text other than a name kept as written", got)
	}
}
