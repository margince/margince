// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAnExtensionFieldWithOptionsIsAPicklistAndTheRestStayText(t *testing.T) {
	recordkinds.Reset()
	t.Cleanup(recordkinds.Reset)
	recordkinds.Register("alpha", []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: "ext_alpha_visit",
		Fields: []extension.RecordKindField{
			{Name: "code", Label: "Code"},
			{Name: "channel", Label: "Channel", Options: []string{"Phone", "Site"}},
		},
		NaturalKey: []string{"code"},
	}})
	reader := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"ext_alpha_visit": {Read: true}}},
	})

	engine, ok := extensionEngine(reader, "ext:alpha:visit")
	if !ok {
		t.Fatal("a reader of the kind got no engine")
	}
	fields := map[string]VocabularyField{}
	for _, f := range extensionVocabulary(engine) {
		fields[f.Name] = f
	}
	if got := fields["channel"]; got.Type != string(storekit.FieldPicklist) || !slices.Equal(got.Options, []string{"Phone", "Site"}) {
		t.Errorf("channel = %+v, want a picklist of Phone and Site", got)
	}
	if got := fields["code"]; got.Type != string(storekit.FieldText) || got.Options != nil {
		t.Errorf("code = %+v, want text with no options", got)
	}
}
