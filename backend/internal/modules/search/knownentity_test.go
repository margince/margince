// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/extregistrytest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func registerAlphaVisit(t *testing.T) {
	t.Helper()
	extregistrytest.Reset(t)
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

// boundPanickingEmbedder is a bound lane whose model call must not happen.
type boundPanickingEmbedder struct{}

func (boundPanickingEmbedder) EmbedIdentity() (string, int) { return "test/embed@3", 3 }

func (boundPanickingEmbedder) Embed(context.Context, model.EmbedRequest) (model.Embeddings, error) {
	panic("Embed must not be called for a kind the embedding table refuses")
}

func TestAReadableExtensionKindIsRefusedBeforeAnyEmbeddingIsPaidFor(t *testing.T) {
	registerAlphaVisit(t)
	ctx := reader("ext_alpha_visit")
	if !knownEntity(ctx, "ext:alpha:visit") {
		t.Fatal("the fixture kind is not readable, so the refusal below would prove nothing")
	}
	_, err := NewStore(nil).UpsertEmbedding(ctx, "ext:alpha:visit", ids.NewV7(), "a visit", boundPanickingEmbedder{})
	if err == nil || !strings.Contains(err.Error(), "unembeddable entity type") {
		t.Fatalf("err = %v, want the kind refused as unembeddable before the stored stamp is read", err)
	}
}
