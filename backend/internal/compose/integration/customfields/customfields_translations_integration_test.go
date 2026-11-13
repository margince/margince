// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	customfieldsmod "github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/labeltranslation"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A field's label and options carry names per UI language. The stored option
// stays the record value, so an options edit drops the names of the options
// it removed and keeps the rest.
func TestACustomFieldNamesItsLabelAndOptionsPerLanguage(t *testing.T) {
	e := integration.Setup(t)
	svc := customfieldsmod.NewService(e.Pool, integration.SchemaPool(t))
	ctx := e.As(e.Rep1, nil, integration.CustomFieldAdminPerms)
	created, err := svc.Create(ctx, customfieldsmod.FieldSpec{
		Object: "deal", Label: "Services", Type: customfieldsmod.TypePicklist,
		Options: []string{"Consulting", "Support"}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(created.Id)

	named, err := svc.Update(ctx, id, customfieldsmod.FieldPatch{
		LabelTranslations: map[string]string{"vi": " Dịch vụ ", "de": ""},
		OptionTranslations: map[string]map[string]string{
			"Consulting": {"vi": "Tư vấn"}, "Support": {"vi": "Hỗ trợ"},
		},
	}, nil)
	if err != nil {
		t.Fatalf("saving the names: %v", err)
	}
	label := labeltranslation.FromWire(named.LabelTranslations)
	if label["vi"] != "Dịch vụ" || len(label) != 1 {
		t.Errorf("label names = %v, want only the trimmed vi name", label)
	}
	if got := labeltranslation.OptionsFromWire(named.OptionTranslations)["Support"]["vi"]; got != "Hỗ trợ" {
		t.Errorf("Support in vi = %q, want Hỗ trợ", got)
	}

	var refused *labeltranslation.Error
	if _, err := svc.Update(ctx, id, customfieldsmod.FieldPatch{
		LabelTranslations: map[string]string{"fr": "Services"},
	}, nil); !errors.As(err, &refused) {
		t.Errorf("a language the UI does not ship answered %v, want the translation refusal", err)
	}
	if _, err := svc.Update(ctx, id, customfieldsmod.FieldPatch{
		OptionTranslations: map[string]map[string]string{"Hosting": {"vi": "Lưu trữ"}},
	}, nil); !errors.As(err, &refused) {
		t.Errorf("an option the field does not offer answered %v, want the translation refusal", err)
	}

	kept, err := svc.SetOptions(ctx, id, []string{"Consulting", "Training"})
	if err != nil {
		t.Fatal(err)
	}
	options := labeltranslation.OptionsFromWire(kept.OptionTranslations)
	if _, stale := options["Support"]; stale || options["Consulting"]["vi"] != "Tư vấn" {
		t.Errorf("after the options edit the names are %v, want Consulting kept and Support dropped", options)
	}
}
