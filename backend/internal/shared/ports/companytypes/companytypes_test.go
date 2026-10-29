// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companytypes

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestTwoUnitsMayDeclareTheSameTypeNameWithoutSharingIt(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	Register("alpha", []extension.CompanyType{{Type: "reseller", Label: "Alpha reseller"}})
	Register("beta", []extension.CompanyType{{Type: "reseller", Label: "Beta reseller"}})

	a, ok := Lookup("ext:alpha:reseller")
	if !ok || a.Unit != "alpha" || a.Label != "Alpha reseller" {
		t.Errorf("alpha lookup = %+v, %v", a, ok)
	}
	b, ok := Lookup("ext:beta:reseller")
	if !ok || b.Unit != "beta" {
		t.Errorf("beta lookup = %+v, %v", b, ok)
	}
	if _, ok := Lookup("ext:gamma:reseller"); ok {
		t.Error("a type no unit declared was found")
	}
	if _, ok := Lookup("reseller"); ok {
		t.Error("a bare name was found")
	}
	if got := All(); len(got) != 2 || got[0].Value != "ext:alpha:reseller" {
		t.Errorf("All() = %+v, want both, ordered by value", got)
	}
}
