// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestRegisterExtensionsRegistersDeclaredLinkTypes(t *testing.T) {
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	err := RegisterExtensions(composableAll([]extension.Extension{{
		Name: "links-ok", Version: "0.0.1",
		LinkTypes: []extension.LinkType{{Type: "follow_up_of", Label: "Follow-up", From: []string{"deal"}, To: []string{"deal"}}},
	}}), nil, nil)
	if err != nil {
		t.Fatalf("RegisterExtensions: %v", err)
	}
	got, ok := recordkinds.LookupLinkType("ext:links-ok:follow_up_of")
	if !ok || got.Unit != "links-ok" || !got.Allows("deal", "deal") {
		t.Fatalf("LookupLinkType = %+v, %v; want the declared type under its unit", got, ok)
	}
}

func TestABadLinkTypeDeclarationAbortsTheBoot(t *testing.T) {
	for _, c := range []struct {
		name  string
		types []extension.LinkType
		want  string
	}{
		{"bad grammar", []extension.LinkType{{Type: "Follow", Label: "x"}}, "valid type name"},
		{"no label", []extension.LinkType{{Type: "follow"}}, "no label"},
		{"bad end", []extension.LinkType{{Type: "follow", Label: "x", To: []string{"project"}}}, "as an end"},
		{"declared twice", []extension.LinkType{{Type: "follow", Label: "a"}, {Type: "follow", Label: "b"}}, "twice"},
	} {
		recordkinds.Reset()
		err := RegisterExtensions(composableAll([]extension.Extension{{
			Name: "links-bad", Version: "0.0.1", LinkTypes: c.types,
		}}), nil, nil)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one naming %q", c.name, err, c.want)
		}
		if _, ok := recordkinds.LookupLinkType("ext:links-bad:follow"); ok {
			t.Errorf("%s: a refused set registered its type", c.name)
		}
	}
	t.Cleanup(recordkinds.Reset)
}
