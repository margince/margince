// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestARemovedKindAndAnUnreadableOneAreBothUnknownToSearch(t *testing.T) {
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("alpha", []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: "ext_alpha_visit",
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}},
		NaturalKey: []string{"code"},
	}})
	reader := func(objects ...string) context.Context {
		grants := map[string]principal.ObjectGrant{}
		for _, object := range objects {
			grants[object] = principal.ObjectGrant{Read: true}
		}
		return principal.WithActor(context.Background(), principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:test",
			Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
		})
	}
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
