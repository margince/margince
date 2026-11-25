// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func registerAlphaVisit(t *testing.T) {
	t.Helper()
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("alpha", []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: "ext_alpha_visit",
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}})
}

func reader(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
	})
}

func TestARemovedKindAndAnUnreadableOneAreBothUnknownToSearch(t *testing.T) {
	registerAlphaVisit(t)
	cases := []struct {
		name string
		ctx  context.Context
		kind string
		want bool
	}{
		{"readable", reader("ext_alpha_visit"), "ext:alpha:visit", true},
		{"declared, no read right", reader("contact"), "ext:alpha:visit", false},
		{"no unit declares it", reader("ext_alpha_visit"), "ext:gone:visit", false},
		{"a core type", reader(), "contact", true},
	}
	for _, c := range cases {
		if got := knownEntity(c.ctx, c.kind); got != c.want {
			t.Errorf("%s: knownEntity = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestASearchOverEveryTypeLeavesOutAKindTheCallerMayNotRead(t *testing.T) {
	registerAlphaVisit(t)
	in := Input{Query: "berlin", WithExtensions: true}
	if types := requestedTypes(reader("contact"), in); slices.Contains(types, "ext:alpha:visit") {
		t.Errorf("types = %v, want the unreadable kind left out rather than refused", types)
	}
	if types := requestedTypes(reader("ext_alpha_visit"), in); !slices.Contains(types, "ext:alpha:visit") {
		t.Errorf("types = %v, want the readable kind in", types)
	}
}
