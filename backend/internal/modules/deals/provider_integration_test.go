// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

// The record seam the MCP door binds, against a real database. Each record is
// written and read back through the seam itself. A green here says a passport
// meets the same deal, catalog and offer the REST door serves.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// eurInstallation prices in EUR and stubs the seams owned by modules this
// package may not import.
func eurInstallation() Installation {
	eur := func(context.Context, pgx.Tx) (string, error) { return "EUR", nil }
	return Installation{
		BaseCurrency: eur, BaseCurrencyApplied: eur,
		Timezone:            func(context.Context, pgx.Tx) (string, error) { return "UTC", nil },
		Name:                func(context.Context, pgx.Tx) (string, error) { return "Margince Test", nil },
		IssuerLegalName:     func(context.Context, pgx.Tx) (string, error) { return "Margince Test GmbH", nil },
		StampCorrespondence: func(context.Context, pgx.Tx, ids.DealID, string) error { return nil },
		ContractVisible:     func(context.Context, string, func(any) int) (string, error) { return "FALSE", nil },
	}
}

type seamEnv struct {
	*configEnv
	provider *Provider
}

func setupSeamEnv(t *testing.T) *seamEnv {
	t.Helper()
	e := setupConfigEnv(t)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	return &seamEnv{configEnv: e, provider: NewProvider(db, eurInstallation())}
}

// passport holds every verb the seam serves for this module.
func (e *seamEnv) passport() context.Context {
	all := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	return humanWith(e.ws, e.admin, map[string]principal.ObjectGrant{
		"deal": all, "offer": all, "product": all, "offer_template": all,
		"pipeline": {Create: true, Read: true, Update: true}, "company": {Read: true},
	})
}

func (e *seamEnv) create(t *testing.T, typ datasource.EntityType, fields string) datasource.EntityRef {
	t.Helper()
	ref, err := e.provider.Create(e.passport(), datasource.CreateInput{
		EntityType: typ, Fields: json.RawMessage(fields), Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating a %s through the seam: %v", typ, err)
	}
	return ref
}

func (e *seamEnv) update(t *testing.T, ref datasource.EntityRef, patch string) {
	t.Helper()
	if _, err := e.provider.Update(e.passport(), datasource.UpdateInput{Ref: ref, Patch: json.RawMessage(patch)}); err != nil {
		t.Fatalf("patching the %s through the seam: %v", ref.Type, err)
	}
}

// readAs reads one record through the seam and decodes it into its wire shape.
func readAs[V any](t *testing.T, e *seamEnv, ref datasource.EntityRef) V {
	t.Helper()
	rec, err := e.provider.Read(e.passport(), ref)
	if err != nil {
		t.Fatalf("reading the %s back through the seam: %v", ref.Type, err)
	}
	var v V
	if err := json.Unmarshal(rec.Fields, &v); err != nil {
		t.Fatalf("decoding the %s: %v", ref.Type, err)
	}
	return v
}

func (e *seamEnv) search(t *testing.T, typ datasource.EntityType, filters map[string]string) []datasource.Record {
	t.Helper()
	rows, _, _, err := e.provider.SearchEntity(e.passport(), typ, nil, 50, nil, filters)
	if err != nil {
		t.Fatalf("listing %s through the seam: %v", typ, err)
	}
	return rows
}

func listsRef(rows []datasource.Record, ref datasource.EntityRef) bool {
	for _, r := range rows {
		if r.Ref == ref {
			return true
		}
	}
	return false
}

// archiveThroughTheSeam runs the stage-time probe, then the archive.
func (e *seamEnv) archiveThroughTheSeam(t *testing.T, ref datasource.EntityRef) {
	t.Helper()
	if err := e.provider.RefuseArchive(e.passport(), ref); err != nil {
		t.Fatalf("the stage-time probe refused a live %s: %v", ref.Type, err)
	}
	if _, err := e.provider.Archive(e.passport(), ref); err != nil {
		t.Fatalf("archiving the %s: %v", ref.Type, err)
	}
}

// refusesASecondArchive holds the probe to the live row: an archived record
// has nothing left for a human to approve archiving.
func (e *seamEnv) refusesASecondArchive(t *testing.T, ref datasource.EntityRef) {
	t.Helper()
	if err := e.provider.RefuseArchive(e.passport(), ref); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("the probe on an archived %s answered %v, want not found", ref.Type, err)
	}
}

func TestTheSeamCarriesADealFromCreateToItsNextStage(t *testing.T) {
	e := setupSeamEnv(t)
	_, from, to := twoStagePipeline(t, e.configEnv)
	semantic, pipelineID, err := e.provider.StageSemantic(e.passport(), from.UUID)
	if err != nil || semantic != string(SemanticOpen) {
		t.Fatalf("the first stage read as %q (%v), want open", semantic, err)
	}
	deal := e.create(t, datasource.EntityDeal, `{"name":"Fleet renewal","pipeline_id":"`+pipelineID.String()+
		`","stage_id":"`+from.String()+`"}`)
	e.update(t, deal, `{"name":"Fleet renewal 2027"}`)
	if got := readAs[crmcontracts.Deal](t, e, deal); got.Name != "Fleet renewal 2027" {
		t.Errorf("the deal read back named %q, want the patched name", got.Name)
	}
	if rows := e.search(t, datasource.EntityDeal, map[string]string{"stage_id": from.String()}); !listsRef(rows, deal) {
		t.Error("the deal is missing from its own stage's list")
	}
	if _, err := e.provider.AdvanceDeal(e.passport(), datasource.AdvanceDealInput{DealID: deal.ID, ToStageID: to.UUID}); err != nil {
		t.Fatalf("advancing the deal: %v", err)
	}
	if got := readAs[crmcontracts.Deal](t, e, deal); got.StageId == nil || ids.UUID(*got.StageId) != to.UUID {
		t.Errorf("the deal sits in stage %v after the advance, want %v", got.StageId, to.UUID)
	}
	e.archiveThroughTheSeam(t, deal)
	if _, err := e.provider.Read(e.passport(), deal); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading the archived deal answered %v, want not found: the seam reads live deals only", err)
	}
}

func TestTheSeamCarriesAProductFromCreateToArchive(t *testing.T) {
	e := setupSeamEnv(t)
	product := e.create(t, datasource.EntityProduct,
		`{"name":"Support seat","currency":"EUR","unit_price_minor":4900,"billing_model":"one_time"}`)
	e.update(t, product, `{"unit_price_minor":5900,"billing_model":"not_specified"}`)
	got := readAs[crmcontracts.Product](t, e, product)
	if got.UnitPriceMinor != 5900 || got.BillingModel != nil {
		t.Errorf("the product read back at %d billed %v, want 5900 and no classification", got.UnitPriceMinor, got.BillingModel)
	}
	if !listsRef(e.search(t, datasource.EntityProduct, map[string]string{"active": "true", "sort": "name"}), product) {
		t.Error("the active product is missing from the active list")
	}
	e.archiveThroughTheSeam(t, product)
	e.refusesASecondArchive(t, product)
	if listsRef(e.search(t, datasource.EntityProduct, nil), product) {
		t.Error("the archived product is still in the default list")
	}
	if !listsRef(e.search(t, datasource.EntityProduct, map[string]string{"include_archived": "true"}), product) {
		t.Error("the archived product is missing even when archived rows are asked for")
	}
	if got := readAs[crmcontracts.Product](t, e, product); got.Name != "Support seat" {
		t.Errorf("the archived product read back as %q, want it still readable", got.Name)
	}
}

func TestTheSeamCarriesAnOfferTemplateFromCreateToArchive(t *testing.T) {
	e := setupSeamEnv(t)
	template := e.create(t, datasource.EntityOfferTemplate,
		`{"name":"Standard","locale":"en-GB","is_default":true,"layout":{"footer":"v1"}}`)
	e.update(t, template, `{"name":"Standard v2","locale":"en-GB","is_default":false,"layout":{"footer":"v2"}}`)
	got := readAs[crmcontracts.OfferTemplate](t, e, template)
	if got.Name != "Standard v2" || got.Layout["footer"] != "v2" || got.IsDefault {
		t.Errorf("the template read back as %q with layout %v, want the full replace", got.Name, got.Layout)
	}
	if !listsRef(e.search(t, datasource.EntityOfferTemplate, map[string]string{"locale": "en-GB"}), template) {
		t.Error("the template is missing from its locale's list")
	}
	if listsRef(e.search(t, datasource.EntityOfferTemplate, map[string]string{"locale": "de-DE"}), template) {
		t.Error("an en-GB template is listed under de-DE")
	}
	e.archiveThroughTheSeam(t, template)
	e.refusesASecondArchive(t, template)
	archived := map[string]string{"include_archived": "true", "sort": "name"}
	if !listsRef(e.search(t, datasource.EntityOfferTemplate, archived), template) {
		t.Error("the archived template is missing even when archived rows are asked for")
	}
}

func TestTheSeamCarriesAnOfferAndItsLines(t *testing.T) {
	e := setupSeamEnv(t)
	dealID, _, _ := twoStagePipeline(t, e.configEnv)
	offer := e.create(t, datasource.EntityOffer, `{"deal_id":"`+dealID.String()+`","currency":"EUR",
		"line_items":[{"description":"Setup","quantity":1,"unit_price_minor":10000}]}`)
	e.update(t, offer, `{"intro_text":"As discussed on Monday."}`)
	if got := readAs[crmcontracts.Offer](t, e, offer); got.IntroText == nil || *got.IntroText != "As discussed on Monday." {
		t.Errorf("the offer's intro read back as %v, want the patched text", got.IntroText)
	}
	if !listsRef(e.search(t, datasource.EntityOffer, map[string]string{"deal_id": dealID.String()}), offer) {
		t.Error("the offer is missing from its deal's list")
	}

	ctx := e.passport()
	added, err := e.provider.AddOfferLine(ctx, offer.ID, json.RawMessage(`{"description":"Training","quantity":2,"unit_price_minor":2500}`))
	if err != nil || added.LineItems == nil || len(*added.LineItems) != 2 {
		t.Fatalf("adding a line answered %v lines (%v), want two", added.LineItems, err)
	}
	line := ids.UUID((*added.LineItems)[1].Id)
	patched, err := e.provider.UpdateOfferLine(ctx, offer.ID, line, json.RawMessage(`{"quantity":4}`))
	if err != nil {
		t.Fatalf("patching the line: %v", err)
	}
	if patched.NetMinor == nil || *patched.NetMinor != 20000 {
		t.Errorf("the offer nets %v after the patch, want 10000 + 4 x 2500", patched.NetMinor)
	}
	removed, err := e.provider.RemoveOfferLine(ctx, offer.ID, line)
	if err != nil || removed.LineItems == nil || len(*removed.LineItems) != 1 {
		t.Fatalf("removing the line answered %v lines (%v), want one", removed.LineItems, err)
	}
	e.archiveThroughTheSeam(t, offer)
	e.refusesASecondArchive(t, offer)
	if got := readAs[crmcontracts.Offer](t, e, offer); got.ArchivedAt == nil {
		t.Error("the archived offer reads back with no archived_at")
	}
}

// An offer list is anchored on its deal, so a deal that does not exist answers
// not found rather than an empty page.
func TestAnOfferListOnAnUnknownDealAnswersNotFound(t *testing.T) {
	e := setupSeamEnv(t)
	_, _, _, err := e.provider.SearchEntity(e.passport(), datasource.EntityOffer, nil, 10, nil,
		map[string]string{"deal_id": ids.NewV7().String()})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("listing the offers of an unknown deal answered %v, want not found", err)
	}
}

func TestTheSeamAnswersNotFoundForARecordThatIsNotThere(t *testing.T) {
	e := setupSeamEnv(t)
	for _, typ := range []datasource.EntityType{
		datasource.EntityProduct, datasource.EntityOfferTemplate, datasource.EntityOffer,
	} {
		if _, err := e.provider.Read(e.passport(), datasource.EntityRef{Type: typ, ID: ids.NewV7()}); !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("reading an unknown %s answered %v, want not found", typ, err)
		}
	}
}
