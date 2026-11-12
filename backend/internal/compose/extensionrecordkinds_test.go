// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func visitKind(unitNS string) extension.RecordKindDeclaration {
	return extension.RecordKindDeclaration{
		Kind: "visit", DisplayName: "Visit", Table: unitNS + "_visit",
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}
}

func TestRegisterExtensionsRegistersDeclaredRecordKinds(t *testing.T) {
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name: "kinds-ok", Version: "0.0.1",
		RecordKinds: []extension.RecordKindDeclaration{visitKind("ext_kinds_ok")},
	}}), nil, nil)
	if err != nil {
		t.Fatalf("RegisterExtensions: %v", err)
	}
	got, ok := recordkinds.Lookup("ext:kinds-ok:visit")
	if !ok || got.Unit != "kinds-ok" || got.Table != "ext_kinds_ok_visit" {
		t.Fatalf("Lookup = %+v, %v; want the declared kind under its unit", got, ok)
	}
}

func TestABadRecordKindDeclarationAbortsTheBoot(t *testing.T) {
	twice := []extension.RecordKindDeclaration{visitKind("ext_kinds_bad"), visitKind("ext_kinds_bad")}
	noKey := visitKind("ext_kinds_bad")
	noKey.NaturalKey = nil
	for _, c := range []struct {
		name  string
		kinds []extension.RecordKindDeclaration
		want  string
	}{
		{"declared twice", twice, "twice"},
		{"no natural key", []extension.RecordKindDeclaration{noKey}, "natural key"},
		{"table of another unit", []extension.RecordKindDeclaration{visitKind("ext_elsewhere")}, "namespace"},
	} {
		recordkinds.Reset()
		err := RegisterExtensions(composableAll([]extension.Extension{{
			Name: "kinds-bad", Version: "0.0.1", RecordKinds: c.kinds,
		}}), nil, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one naming %q", c.name, err, c.want)
		}
		if got := recordkinds.All(); len(got) != 0 {
			t.Errorf("%s: %d kinds registered by a refused set", c.name, len(got))
		}
	}
	t.Cleanup(recordkinds.Reset)
}

func TestARecordKindGatesOnItsOwnRbacObjectWithNoGrantByDefault(t *testing.T) {
	exts := []extension.Extension{{
		Name: "kinds-rbac", Version: "0.0.1",
		RecordKinds: []extension.RecordKindDeclaration{visitKind("ext_kinds_rbac")},
	}}
	objects, defaults := recordKindRbac(exts)
	if len(objects) != 1 || string(objects[0]) != "ext_kinds_rbac_visit" {
		t.Errorf("objects = %v, want the kind's own object", objects)
	}
	if len(defaults) != 0 {
		t.Errorf("defaults = %v, want none: a kind declaring no defaults denies every role", defaults)
	}

	exts[0].RecordKinds[0].Defaults = []extension.RbacDefault{{Role: "rep", Actions: []extension.RbacAction{extension.RbacRead}}}
	_, defaults = recordKindRbac(exts)
	if len(defaults) != 1 || defaults[0].Role != "rep" || defaults[0].Actions[0] != "read" {
		t.Errorf("defaults = %+v, want the one declared grant", defaults)
	}
}
