// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Text the server builds names stages and custom field choices in the
// reader's UI language, through labeltranslation.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// wonStageOf is the won stage of a pipeline.
func wonStageOf(t *testing.T, e *Env, pipeline ids.UUID) ids.StageID {
	t.Helper()
	id := ids.From[ids.PipelineKind](pipeline)
	stages, err := e.Deals.ListStages(e.Admin(), &id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range stages {
		if stage.Semantic == "won" {
			return ids.From[ids.StageKind](ids.UUID(stage.Id))
		}
	}
	t.Fatal("the pipeline has no won stage")
	return ids.StageID{}
}

// reading is ctx as a request whose screen shows locale, as the display
// locale middleware leaves it. The principal's own choice says English, so a
// read that ignored the screen would answer in English.
func reading(ctx context.Context, t *testing.T, locale string) context.Context {
	t.Helper()
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the context carries no principal")
	}
	actor.Locale = "en"
	return systemtext.WithLocale(principal.WithActor(ctx, actor), textlang.Lang(locale))
}

func TestTheCompanyPageAndTheMoveConfirmationNameStagesInTheReadersLanguage(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Globex", nil)
	first, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](e.SeedOpenDealOn(t, "Globex renewal", company)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	open, won := ids.From[ids.StageKind](ids.UUID(*first.StageId)), wonStageOf(t, e, ids.UUID(*first.PipelineId))
	if _, err := e.Deals.UpdateStage(e.Admin(), open, deals.UpdateStageInput{NameTranslations: map[string]string{"vi": "Cơ hội"}}); err != nil {
		t.Fatal(err)
	}
	stageOf := func(locale string) string {
		t.Helper()
		page, err := companySurfaceService(e).Assemble(reading(e.Admin(), t, locale), companyIDOf(company))
		if err != nil {
			t.Fatal(err)
		}
		if page.Deals == nil || len(page.Deals.Data) != 1 || page.Deals.Data[0].StageName == nil {
			t.Fatalf("the company page lists %+v, want the one open deal with its stage", page.Deals)
		}
		return *page.Deals.Data[0].StageName
	}
	stored, err := e.Deals.GetStage(e.Admin(), open)
	if err != nil {
		t.Fatal(err)
	}
	if got := stageOf("vi"); got != "Cơ hội" {
		t.Errorf("a Vietnamese reader's company page names the stage %q, want Cơ hội", got)
	}
	if got := stageOf("en"); got != stored.Name {
		t.Errorf("an English reader's company page names the stage %q, want the stored %q", got, stored.Name)
	}

	// The confirmation a move answers names the stage the deal moved to.
	companyID := ids.From[ids.CompanyKind](company)
	second, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Second deal", PipelineID: ids.From[ids.PipelineKind](ids.UUID(*first.PipelineId)), StageID: open, CompanyID: &companyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	moved := ids.From[ids.DealKind](ids.UUID(second.Id))
	if _, err := e.Deals.UpdateStage(e.Admin(), won, deals.UpdateStageInput{NameTranslations: map[string]string{"vi": "Đã thắng"}}); err != nil {
		t.Fatal(err)
	}
	reason := "verbal"
	advanced, err := e.Deals.AdvanceDeal(reading(e.Admin(), t, "vi"), moved, deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: &reason})
	if err != nil {
		t.Fatal(err)
	}
	if advanced.Probability == nil || advanced.Probability.StageName != "Đã thắng" {
		t.Errorf("the move confirmation reads %+v, want the stage named Đã thắng", advanced.Probability)
	}
}

func TestReportableChoicesCarryTheReadersNames(t *testing.T) {
	e := Setup(t)
	svc := customfields.NewService(e.Pool, SchemaPool(t))
	admin := e.As(e.Rep1, nil, CustomFieldAdminPerms)
	created, err := svc.Create(admin, customfields.FieldSpec{
		Object: "deal", Label: "Services", Type: customfields.TypePicklist,
		Options: []string{"Consulting", "Support"}, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	reportable := true
	if _, err := svc.Update(admin, ids.UUID(created.Id), customfields.FieldPatch{
		Reportable:         &reportable,
		LabelTranslations:  map[string]string{"vi": "Dịch vụ"},
		OptionTranslations: map[string]map[string]string{"Consulting": {"vi": "Tư vấn"}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	read := func(locale string) (string, map[string]string) {
		t.Helper()
		var label string
		var named map[string]string
		ctx := reading(e.Admin(), t, locale)
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			cols, err := customfields.ReportableColumnsIn(ctx, tx, "deal")
			if err != nil || len(cols) != 1 {
				t.Fatalf("reportable columns = %v (%v), want the one field", cols, err)
			}
			label, named = cols[0].Label, cols[0].OptionLabels
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return label, named
	}
	if label, named := read("vi"); label != "Dịch vụ" || named["Consulting"] != "Tư vấn" || named["Support"] != "" {
		t.Errorf("a Vietnamese reader gets %q and %v, want Dịch vụ and only Consulting named", label, named)
	}
	if label, named := read("en"); label != "Services" || len(named) != 0 {
		t.Errorf("an English reader gets %q and %v, want the stored label and no names", label, named)
	}
}
