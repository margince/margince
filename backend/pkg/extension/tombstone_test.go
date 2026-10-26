// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"encoding/json"
	"testing"
)

func TestATombstoneAnswersOnlyIDAndKind(t *testing.T) {
	raw, err := json.Marshal(Tombstone{ID: "7d1c", Kind: "deal"})
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"id":"7d1c","kind":"deal"}`; string(raw) != want {
		t.Fatalf("tombstone %s, want %s", raw, want)
	}
}
