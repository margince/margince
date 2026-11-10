// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"testing"
)

// A contact still named by its address takes the name its parts spell. A part
// the row does not map keeps the stored one, so a partial mapping never drops it.
func TestARowNamingOnePartKeepsTheStoredOther(t *testing.T) {
	stored := map[string]json.RawMessage{
		fieldFullName: json.RawMessage(`"jane@acme.test"`),
		"last_name":   json.RawMessage(`"Doe"`),
		"emails":      json.RawMessage(`[{"email":"jane@acme.test","is_primary":true}]`),
	}
	got := withNamePartsOverStoredEmail(stored, map[string]string{"first_name": "Jane"})
	if got[fieldFullName] != "Jane Doe" {
		t.Fatalf("full name %q, want the row's first name with the stored last name", got[fieldFullName])
	}
	both := withNamePartsOverStoredEmail(stored, map[string]string{"first_name": "Jane", "last_name": "Smith"})
	if both[fieldFullName] != "Jane Smith" {
		t.Fatalf("full name %q, want the row's two parts", both[fieldFullName])
	}
	named := map[string]json.RawMessage{fieldFullName: json.RawMessage(`"Jane Doe"`), "last_name": json.RawMessage(`"Doe"`)}
	if _, sent := withNamePartsOverStoredEmail(named, map[string]string{"first_name": "Janet"})[fieldFullName]; sent {
		t.Fatal("a contact with a chosen name was renamed by a row")
	}
}
