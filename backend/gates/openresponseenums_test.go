// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// Every enum a response can carry is open, and every enum only a request
// carries stays closed. The rule and the classification live in
// internal/platform/openenums, which contract-breaking-check runs too; this
// gate holds api/crm.yaml to it on every change.

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/openenums"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestEveryResponseEnumIsOpenAndEveryRequestEnumIsClosed(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(contractDocument)
	if err != nil {
		t.Fatalf("reading %s: %v", contractDocument, err)
	}
	enums, err := openenums.Classify(raw)
	if err != nil {
		t.Fatal(err)
	}
	var responses, requests int
	for _, e := range enums {
		if e.Response {
			responses++
		} else {
			requests++
		}
	}
	if responses == 0 || requests == 0 {
		t.Fatalf("classified %d response and %d request enums; an empty side means the walk read nothing", responses, requests)
	}
	for _, violation := range openenums.Violations(contractDocument, enums) {
		t.Error(violation + " (go run ./tools/check-openenums -contract api/crm.yaml -mark)")
	}
}
