// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordkinds

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func declared(kind, table string) extension.RecordKindDeclaration {
	return extension.RecordKindDeclaration{
		Kind: kind, DisplayName: "Visit", Table: table,
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}
}

func TestTwoUnitsMayDeclareTheSameKindWithoutSharingIt(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	Register("alpha", []extension.RecordKindDeclaration{declared("visit", "ext_alpha_visit")})
	Register("beta", []extension.RecordKindDeclaration{declared("visit", "ext_beta_visit")})

	a, ok := Lookup("ext:alpha:visit")
	if !ok || a.Unit != "alpha" || a.Table != "ext_alpha_visit" || a.Object != "ext_alpha_visit" {
		t.Errorf("alpha lookup = %+v, %v", a, ok)
	}
	if b, ok := Lookup("ext:beta:visit"); !ok || b.Table != "ext_beta_visit" {
		t.Errorf("beta lookup = %+v, %v", b, ok)
	}
	for _, name := range []string{"ext:gamma:visit", "visit", "contact", ""} {
		if _, ok := Lookup(name); ok {
			t.Errorf("Lookup(%q) found a kind no unit declared", name)
		}
	}
	if got := All(); len(got) != 2 || got[0].Name != "ext:alpha:visit" {
		t.Errorf("All() = %+v, want both, ordered by name", got)
	}
}

func TestDeletePolicyDefaultsToArchiveOnly(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	Register("alpha", []extension.RecordKindDeclaration{declared("visit", "ext_alpha_visit")})
	if d, _ := Lookup("ext:alpha:visit"); d.DeletePolicy != extension.DeleteArchiveOnly {
		t.Errorf("DeletePolicy = %q, want %q", d.DeletePolicy, extension.DeleteArchiveOnly)
	}
}

func TestIsExtensionName(t *testing.T) {
	for name, want := range map[string]bool{"ext:a:b": true, "ext:": true, "contact": false, "": false} {
		if got := IsExtensionName(name); got != want {
			t.Errorf("IsExtensionName(%q) = %v, want %v", name, got, want)
		}
	}
}
