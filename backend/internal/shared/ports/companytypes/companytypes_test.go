// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companytypes

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestTwoUnitsMayDeclareTheSameTypeNameWithoutSharingIt(t *testing.T) {
	t.Parallel()
	reg := Of([]extension.Extension{
		{Name: "alpha", CompanyTypes: []extension.CompanyType{{Type: "reseller", Label: "Alpha reseller"}}},
		{Name: "beta", CompanyTypes: []extension.CompanyType{{Type: "reseller", Label: "Beta reseller"}}},
	})

	a, ok := reg.Lookup("ext:alpha:reseller")
	if !ok || a.Unit != "alpha" || a.Label != "Alpha reseller" {
		t.Errorf("alpha lookup = %+v, %v", a, ok)
	}
	b, ok := reg.Lookup("ext:beta:reseller")
	if !ok || b.Unit != "beta" {
		t.Errorf("beta lookup = %+v, %v", b, ok)
	}
	if _, ok := reg.Lookup("ext:gamma:reseller"); ok {
		t.Error("a type no unit declared was found")
	}
	if _, ok := reg.Lookup("reseller"); ok {
		t.Error("a bare name was found")
	}
	if got := reg.All(); len(got) != 2 || got[0].Value != "ext:alpha:reseller" {
		t.Errorf("All() = %+v, want both, ordered by value", got)
	}
}

func TestTheZeroRegistryDeclaresNothing(t *testing.T) {
	t.Parallel()
	var reg Registry
	if _, ok := reg.Lookup("ext:alpha:reseller"); ok {
		t.Error("the zero registry found a type")
	}
	if got := reg.All(); got == nil || len(got) != 0 {
		t.Errorf("All() = %#v, want an empty, non-nil list", got)
	}
}
