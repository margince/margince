// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func TestRegisterExtensionsRegistersDeclaredCompanyTypes(t *testing.T) {
	composed, err := RegisterExtensions(composableAll([]extension.Extension{{
		Name: "types-ok", Version: "0.0.1",
		CompanyTypes: []extension.CompanyType{{Type: "reseller", Label: "Reseller"}},
	}}), nil, nil)
	if err != nil {
		t.Fatalf("RegisterExtensions: %v", err)
	}
	got, ok := composed.CompanyTypes().Lookup("ext:types-ok:reseller")
	if !ok || got.Unit != "types-ok" || got.Label != "Reseller" {
		t.Fatalf("Lookup = %+v, %v; want the declared type under its unit", got, ok)
	}
	if _, ok := installedCompanyTypes().Lookup("ext:types-ok:reseller"); !ok {
		t.Fatal("the record provider's registry does not hold the type the composition declares")
	}
}

func TestABadCompanyTypeDeclarationAbortsTheBoot(t *testing.T) {
	for _, c := range []struct {
		name  string
		types []extension.CompanyType
		want  string
	}{
		{"bad grammar", []extension.CompanyType{{Type: "Re-Seller", Label: "x"}}, "valid type name"},
		{"no label", []extension.CompanyType{{Type: "reseller"}}, "no label"},
		{"declared twice", []extension.CompanyType{{Type: "reseller", Label: "a"}, {Type: "reseller", Label: "b"}}, "twice"},
	} {
		composed, err := RegisterExtensions(composableAll([]extension.Extension{{
			Name: "types-bad", Version: "0.0.1", CompanyTypes: c.types,
		}}), nil, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one naming %q", c.name, err, c.want)
		}
		if composed != nil {
			t.Errorf("%s: a refused set answered a composition", c.name)
		}
		if _, ok := installedCompanyTypes().Lookup("ext:types-bad:reseller"); ok {
			t.Errorf("%s: a refused set installed its types", c.name)
		}
	}
}
