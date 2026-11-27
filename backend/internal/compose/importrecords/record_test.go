// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importrecords

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func codeOf(problems []*CellProblem) []migration.ReasonCode {
	var codes []migration.ReasonCode
	for _, problem := range problems {
		codes = append(codes, problem.Why.Code)
	}
	return codes
}

func TestAProjectRowIsBornInAnOpenPhaseWithDatesInOrder(t *testing.T) {
	cases := map[string]struct {
		fields map[string]string
		want   []migration.ReasonCode
	}{
		"a start phase":         {map[string]string{ProjectPhase: " Pursuing "}, nil},
		"closed":                {map[string]string{ProjectPhase: "closed"}, []migration.ReasonCode{migration.ReasonProjectPhaseClosed}},
		"an unknown phase":      {map[string]string{ProjectPhase: "someday"}, []migration.ReasonCode{migration.ReasonProjectPhaseUnknown}},
		"an unreadable date":    {map[string]string{ProjectStartedAt: "01.09.2026"}, []migration.ReasonCode{migration.ReasonNotADate}},
		"the end before start":  {map[string]string{ProjectStartedAt: "2026-09-01", ProjectTargetEndDate: "2026-08-31"}, []migration.ReasonCode{migration.ReasonProjectDatesInverted}},
		"the end on the start":  {map[string]string{ProjectStartedAt: "2026-09-01", ProjectTargetEndDate: "2026-09-01"}, nil},
		"an end with no start":  {map[string]string{ProjectTargetEndDate: "2020-01-01"}, nil},
		"an empty phase column": {map[string]string{ProjectPhase: ""}, nil},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			typed, problems := projectRecord{}.CheckCells(tc.fields, nil)
			got := codeOf(problems)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Fatalf("problems = %v, want %v", got, tc.want)
			}
			if tc.want == nil && typed == nil {
				t.Fatal("typed = nil, want the checked cells")
			}
		})
	}
	typed, _ := projectRecord{}.CheckCells(map[string]string{ProjectPhase: " Pursuing "}, nil)
	if typed[ProjectPhase] != "pursuing" {
		t.Fatalf("phase = %v, want the store's spelling", typed[ProjectPhase])
	}
}

func TestANewProjectNeedsANameAndACompanyTheUploadMayStillCreate(t *testing.T) {
	company := ids.NewV7().String()
	named := map[string]string{ProjectName: "Rollout"}
	cases := map[string]struct {
		fields map[string]string
		typed  map[string]any
		want   migration.ReasonCode
	}{
		"complete":                   {named, map[string]any{targetCompanyRef: company}, ""},
		"no name":                    {map[string]string{ProjectName: "  "}, map[string]any{targetCompanyRef: company}, migration.ReasonProjectUnnamed},
		"no company":                 {named, nil, migration.ReasonProjectCompanyMissing},
		"a company the upload makes": {named, map[string]any{targetCompanyRef: PendingLink}, ""},
	}
	for name, tc := range cases {
		if got := (projectRecord{}).RefuseNew(tc.fields, tc.typed).Code; got != tc.want {
			t.Errorf("%s: refusal = %q, want %q", name, got, tc.want)
		}
	}
}

func TestAProjectRowCannotMoveTheProjectToAnotherCompanyOrInvertItsDates(t *testing.T) {
	home, away := ids.NewV7(), ids.NewV7()
	stored, err := json.Marshal(map[string]any{"company_id": home.String(), "started_at": "2026-09-01", "target_end_date": "2027-03-31"})
	if err != nil {
		t.Fatal(err)
	}
	record := projectRecord{}
	if why := record.RefuseChange(stored, nil, map[string]any{targetCompanyRef: away.String()}); why.Code != migration.ReasonProjectCompanyFixed {
		t.Errorf("another company: reason = %q, want %q", why.Code, migration.ReasonProjectCompanyFixed)
	}
	if why := record.RefuseChange(stored, nil, map[string]any{targetCompanyRef: home.String()}); why.Code != "" {
		t.Errorf("the same company: reason = %q, want none", why.Code)
	}
	typed, problems := record.CheckCells(map[string]string{ProjectTargetEndDate: "2026-08-01"}, nil)
	if problems != nil {
		t.Fatalf("problems = %v", problems)
	}
	if why := record.RefuseChange(stored, map[string]string{ProjectTargetEndDate: "2026-08-01"}, typed); why.Code != migration.ReasonProjectDatesInverted {
		t.Errorf("an end before the stored start: reason = %q, want %q", why.Code, migration.ReasonProjectDatesInverted)
	}
}

func TestAProductPriceNeedsItsCurrencyAndEveryOtherCellMustRead(t *testing.T) {
	cases := map[string]struct {
		fields map[string]string
		want   []migration.ReasonCode
	}{
		"a price in a currency":  {map[string]string{ProductUnitPrice: "12.50", ProductCurrency: "eur"}, nil},
		"a price with none":      {map[string]string{ProductUnitPrice: "12.50"}, []migration.ReasonCode{migration.ReasonAmountNeedsCurrency}},
		"a price in words":       {map[string]string{ProductUnitPrice: "ten", ProductCurrency: "EUR"}, []migration.ReasonCode{migration.ReasonNotAnAmount}},
		"a rate in words":        {map[string]string{ProductDefaultTaxRate: "high"}, []migration.ReasonCode{migration.ReasonNotANumber}},
		"a rate with a percent":  {map[string]string{ProductDefaultTaxRate: "19%"}, nil},
		"an interval in words":   {map[string]string{ProductBillingInterval: "yearly"}, []migration.ReasonCode{migration.ReasonNotANumber}},
		"an active flag in text": {map[string]string{ProductActive: "maybe"}, []migration.ReasonCode{migration.ReasonNotYesNo}},
	}
	for name, tc := range cases {
		_, problems := productRecord{}.CheckCells(tc.fields, nil)
		if got := codeOf(problems); fmt.Sprint(got) != fmt.Sprint(tc.want) {
			t.Errorf("%s: problems = %v, want %v", name, got, tc.want)
		}
	}
	typed, _ := productRecord{}.CheckCells(map[string]string{ProductUnitPrice: "12.50", ProductCurrency: "eur", ProductActive: "no"}, nil)
	if typed[ProductUnitPrice] != int64(1250) || typed[ProductCurrency] != "EUR" || typed[ProductActive] != false {
		t.Fatalf("typed = %v, want the price in cents, the code upper-cased and the flag read", typed)
	}
}

func TestAChangeToTheBillingModelRestatesItsInterval(t *testing.T) {
	recurring, twelve := crmcontracts.ProductBillingModel("recurring"), crmcontracts.ProductBillingIntervalMonths(12)
	stored := crmcontracts.Product{BillingModel: &recurring, BillingIntervalMonths: &twelve}
	oneOff := productRecord{}.updateFrom(stored, map[string]string{ProductBillingModel: "one_time"}, nil)
	if !oneOff.Classified || oneOff.BillingModel == nil || *oneOff.BillingModel != "one_time" || oneOff.BillingIntervalMonths != nil {
		t.Errorf("a move to one-off = %+v, want the stored interval dropped", oneOff)
	}
	moved := productRecord{}.updateFrom(stored, map[string]string{ProductBillingInterval: "3"}, map[string]any{ProductBillingInterval: 3})
	if !moved.Classified || *moved.BillingModel != "recurring" || *moved.BillingIntervalMonths != 3 {
		t.Errorf("a new interval = %+v, want the stored model kept beside it", moved)
	}
	if untouched := (productRecord{}).updateFrom(stored, map[string]string{ProductName: "Renamed"}, nil); untouched.Classified {
		t.Errorf("a rename = %+v, want the classification left alone", untouched)
	}
	oneTime := crmcontracts.ProductBillingModel("one_time")
	oneOffStored := mustJSON(t, crmcontracts.Product{BillingModel: &oneTime})
	if why := (productRecord{}).RefuseChange(oneOffStored, map[string]string{ProductBillingModel: "recurring"}, nil); why.Code != migration.ReasonValueRefused {
		t.Errorf("a recurring price with no interval: reason = %q, want %q", why.Code, migration.ReasonValueRefused)
	}
}

func mustJSON(t *testing.T, value crmcontracts.Product) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestAProductRefusalIsReportedAsTheRowsOwnCell(t *testing.T) {
	record := productRecord{}
	fields := map[string]string{ProductSKU: "SUP-1", ProductUnitPrice: "-5"}
	if why, refused := record.Refusal(fmt.Errorf("sku already in use: %w", apperrors.ErrConflict), fields); !refused ||
		why.Code != migration.ReasonProductSKUHeld || why.Params["value"] != "SUP-1" {
		t.Errorf("a held SKU = %v, %v, want product_sku_held naming the SKU", why, refused)
	}
	if why, refused := record.Refusal(deals.RefuseProductUpdate(crmcontracts.Product{}, deals.UpdateProductInput{UnitPriceMinor: new(int64(-5))}), fields); !refused ||
		why.Code != migration.ReasonValueRefused || why.Params["value"] != "-5" {
		t.Errorf("a price below zero = %v, %v, want value_refused naming the cell the file wrote", why, refused)
	}
	if _, refused := record.Refusal(errors.New("connection reset"), fields); refused {
		t.Error("an unrelated error was read as a bad row, and would not stop the run")
	}
}

// Every target a project or product file may map reaches the store's create
// input. It reaches the update input too, unless the store takes it at creation
// only. A target the import accepted and then dropped is worse than one it never offered.
// A new target in the lists fails here until it is placed.
func TestEveryProjectAndProductTargetReachesTheStoresInput(t *testing.T) {
	owner := ids.NewV7()
	company, deal := ids.NewV7().String(), ids.NewV7().String()
	const (
		birthOnly = "written when the row creates the record"
		both      = "written on create and on update"
		elsewhere = "applied by the writer beside the store call"
	)
	projectKinds := map[string]string{
		ProjectName: both, ProjectPhase: birthOnly, targetCompanyRef: birthOnly, ProjectDealRef: birthOnly,
		targetOwner: both, targetDescribed: both, ProjectStartedAt: both, ProjectTargetEndDate: both, "tags": elsewhere,
	}
	for _, target := range ProjectTargets {
		if projectKinds[target] == "" {
			t.Errorf("project target %q is not placed: say whether it reaches the create input, the update input, or the writer", target)
		}
	}
	fields := map[string]string{}
	for _, target := range ProjectTargets {
		fields[target] = "value"
	}
	typed := map[string]any{
		targetOwner: ids.UUID(owner), targetCompanyRef: company, ProjectDealRef: deal, ProjectPhase: "pursuing",
		ProjectStartedAt: openapiDay("2026-09-01"), ProjectTargetEndDate: openapiDay("2026-10-01"),
	}
	record := projectRecord{}
	in, up := record.birthOf(fields, typed, "src", new("ref")), record.updateFrom(fields, typed)
	created := map[string]bool{
		ProjectName: in.Name != "", ProjectPhase: in.Phase != "", targetCompanyRef: !in.CompanyID.IsZero(), ProjectDealRef: in.DealID != nil,
		targetOwner: in.OwnerID != nil, targetDescribed: in.Description != nil, ProjectStartedAt: in.StartedAt != nil, ProjectTargetEndDate: in.TargetEndDate != nil,
	}
	updated := map[string]bool{
		ProjectName: up.Name != nil, targetOwner: up.OwnerID != nil, targetDescribed: up.Description != nil,
		ProjectStartedAt: up.StartedAt != nil, ProjectTargetEndDate: up.TargetEndDate != nil,
	}
	for target, kind := range projectKinds {
		if kind != elsewhere && !created[target] {
			t.Errorf("project target %q does not reach the create input", target)
		}
		if kind == both && !updated[target] {
			t.Errorf("project target %q does not reach the update input", target)
		}
		if kind == birthOnly && updated[target] {
			t.Errorf("project target %q reaches the update input, which the store cannot take", target)
		}
	}

	productFields := map[string]string{}
	for _, target := range ProductTargets {
		productFields[target] = "value"
	}
	productTyped := map[string]any{
		ProductUnitPrice: int64(100), ProductCurrency: "EUR", ProductDefaultTaxRate: 19.0, ProductBillingInterval: 12, ProductActive: true,
	}
	paid := productRecord{}.birthOf(productFields, productTyped)
	recurring := crmcontracts.ProductBillingModel("recurring")
	patch := productRecord{}.updateFrom(crmcontracts.Product{BillingModel: &recurring}, productFields, productTyped)
	createdProduct := map[string]bool{
		ProductName: paid.Name != "", ProductSKU: paid.SKU != nil, targetDescribed: paid.Description != nil, ProductUnit: paid.Unit != nil,
		ProductUnitPrice: paid.UnitPriceMinor != 0, ProductCurrency: paid.Currency != "", ProductDefaultTaxRate: paid.DefaultTaxRate != nil,
		ProductTaxCategory: paid.TaxCategory != nil, ProductBillingModel: paid.BillingModel != nil, ProductBillingInterval: paid.BillingIntervalMonths != nil,
		ProductActive: paid.Active != nil,
	}
	updatedProduct := map[string]bool{
		ProductName: patch.Name != nil, ProductSKU: patch.SKU != nil, targetDescribed: patch.Description != nil, ProductUnit: patch.Unit != nil,
		ProductUnitPrice: patch.UnitPriceMinor != nil, ProductCurrency: patch.Currency != nil, ProductDefaultTaxRate: patch.DefaultTaxRate != nil,
		ProductTaxCategory: patch.TaxCategory != nil, ProductBillingModel: patch.Classified && patch.BillingModel != nil,
		ProductBillingInterval: patch.Classified && patch.BillingIntervalMonths != nil, ProductActive: patch.Active != nil,
	}
	for _, target := range ProductTargets {
		if !createdProduct[target] {
			t.Errorf("product target %q does not reach the create input", target)
		}
		if !updatedProduct[target] {
			t.Errorf("product target %q does not reach the update input", target)
		}
	}
}

func openapiDay(text string) openapi_types.Date {
	date, _ := day(text)
	return openapi_types.Date{Time: date}
}

func TestADealNamedByARefAndByAnIDIsOneClaim(t *testing.T) {
	deal := ids.NewV7()
	byID := ProjectDealKey(map[string]string{ProjectDealRef: deal.String()}, map[string]any{ProjectDealRef: deal.String()})
	byRef := ProjectDealKey(map[string]string{ProjectDealRef: "D-1"}, map[string]any{ProjectDealRef: deal.String()})
	if byID == "" || byID != byRef {
		t.Fatalf("claims = %q and %q, want one key for the deal both rows resolve to", byID, byRef)
	}
	if got := ProjectDealKey(map[string]string{ProjectDealRef: " D-2 "}, map[string]any{ProjectDealRef: PendingLink}); got != "pending:D-2" {
		t.Fatalf("a deal the upload creates = %q, want it keyed by the cell that names it", got)
	}
}
