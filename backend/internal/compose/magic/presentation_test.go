// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func dealRow(before, after string) entry {
	e := auditRow("system:close-date", before, after, "", whenStaged)
	e.EntityType = typeDeal
	return e
}

func changeOf(t *testing.T, changes []crmcontracts.MagicFieldChange, field string) crmcontracts.MagicFieldChange {
	t.Helper()
	for _, c := range changes {
		if c.Field == field {
			return c
		}
	}
	t.Fatalf("no change to %s among %+v", field, changes)
	return crmcontracts.MagicFieldChange{}
}

func TestAnUpdateOfInternalColumnsAloneIsHousekeeping(t *testing.T) {
	fingerprint := auditRow("system:capture_classify",
		`{"agenda_fingerprint": "a1", "updated_at": "2026-11-01T10:00:00Z"}`,
		`{"agenda_fingerprint": "b2", "updated_at": "2026-11-02T10:00:00Z"}`, "", whenStaged)
	fingerprint.EntityType = typeActivity
	provisional := dealRow(`{"close_date_provisional": true, "win_probability_set_by": "margince"}`,
		`{"close_date_provisional": false, "win_probability_set_by": "human"}`)

	lines, housekeeping := linesOf(unmasked(t), []entry{fingerprint, provisional}, nil, 100)
	if len(lines) != 0 || housekeeping != 2 {
		t.Fatalf("got %d lines and %d housekeeping, want no line and both rows counted as housekeeping", len(lines), housekeeping)
	}
	for _, e := range []entry{fingerprint, provisional} {
		if changes := changesOf(e); len(changes) != 0 {
			t.Errorf("an internal-only update listed changes: %+v", changes)
		}
	}
}

func TestAStageMoveWithInternalColumnsShowsOnlyTheStage(t *testing.T) {
	row := dealRow(
		`{"stage_id": "5ea9e000-0000-4000-8000-000000000001", "close_date_provisional": true, "win_probability_set_by": "margince", "pipeline_id": "91be0000-0000-4000-8000-000000000001"}`,
		`{"stage_id": "5ea9e000-0000-4000-8000-000000000002", "close_date_provisional": false, "win_probability_set_by": "human", "pipeline_id": "91be0000-0000-4000-8000-000000000002"}`)

	described, ok := describe(row)
	if !ok || (*described.summary.Values)["fields"] != "stage id" {
		t.Fatalf("the sentence is %+v, want it to name the stage alone", described.summary)
	}
	changes := changesOf(row)
	if len(changes) != 1 || changes[0].Field != "stage_id" || changes[0].Type == nil || *changes[0].Type != crmcontracts.MagicFieldChangeTypeStage {
		t.Fatalf("changes = %+v, want the stage alone, read as a stage", changes)
	}
	line, _, _ := lineOf(unmasked(t), row)
	for _, internal := range []string{"close_date_provisional", "win_probability_set_by", "pipeline_id"} {
		if _, shown := (*line.After)[internal]; shown {
			t.Errorf("the line's image carries %s", internal)
		}
	}
}

func TestAColumnNobodyClassifiedIsNotShown(t *testing.T) {
	row := dealRow(`{"name": "Old", "brand_new_column": 1}`, `{"name": "New", "brand_new_column": 2}`)
	changes := changesOf(row)
	if len(changes) != 1 || changes[0].Field != "name" {
		t.Fatalf("changes = %+v, want the name alone", changes)
	}
	if presentedField(typeDeal, "brand_new_column") {
		t.Fatal("an unclassified column is presented")
	}
	if !presentedField(typeDeal, "cf_tier") {
		t.Fatal("a custom field is not presented")
	}
}

func TestEachPresentedValueSaysHowItIsRead(t *testing.T) {
	row := dealRow(
		`{"expected_close_date": "2026-11-30", "owner_id": "0e0e0000-0000-4000-8000-000000000001", "status": "open", "name": "Old"}`,
		`{"expected_close_date": "2026-12-15", "owner_id": "0e0e0000-0000-4000-8000-000000000002", "status": "won", "name": "New"}`)
	changes := changesOf(row)
	want := map[string]crmcontracts.MagicFieldChangeType{
		"expected_close_date": crmcontracts.MagicFieldChangeTypeDate,
		"owner_id":            crmcontracts.MagicFieldChangeTypeMember,
		"status":              crmcontracts.MagicFieldChangeTypeEnum,
		"name":                crmcontracts.MagicFieldChangeTypeText,
	}
	for field, kind := range want {
		c := changeOf(t, changes, field)
		if c.Type == nil || *c.Type != kind {
			t.Errorf("%s reads as %v, want %s", field, c.Type, kind)
		}
	}
	date := changeOf(t, changes, "expected_close_date")
	if date.Precision == nil || *date.Precision != crmcontracts.MagicFieldChangePrecisionDay {
		t.Errorf("a close date has precision %v, want a calendar day", date.Precision)
	}
}

func TestAnAmountCarriesItsCurrency(t *testing.T) {
	withCurrency := changeOf(t, changesOf(dealRow(
		`{"amount_minor": 1200000, "currency": "EUR"}`, `{"amount_minor": 1500000, "currency": "EUR"}`)), "amount_minor")
	if withCurrency.Type == nil || *withCurrency.Type != crmcontracts.MagicFieldChangeTypeMoney ||
		withCurrency.Currency == nil || *withCurrency.Currency != "EUR" || withCurrency.After != float64(1500000) {
		t.Fatalf("amount = %+v, want minor units in EUR", withCurrency)
	}

	alone := dealRow(`{"amount_minor": 1200000}`, `{"amount_minor": 1500000}`)
	held := map[ids.UUID]heldCurrency{alone.EntityID: {code: "CHF", setAt: whenStaged.Add(-time.Hour)}}
	if c := changeOf(t, typedChangesOf(alone, held), "amount_minor"); c.Currency == nil || *c.Currency != "CHF" {
		t.Fatalf("amount = %+v, want the currency the deal held", c)
	}
	held[alone.EntityID] = heldCurrency{code: "CHF", setAt: whenStaged.Add(time.Hour)}
	if c := changeOf(t, typedChangesOf(alone, held), "amount_minor"); c.Currency != nil {
		t.Fatalf("amount = %+v, want no currency once a later write set another", c)
	}
}

// stubNames answers the names a reader may read; any other id is absent.
type stubNames map[string]map[ids.UUID]string

func (s stubNames) Labels(_ context.Context, entityType string, want []ids.UUID) (map[ids.UUID]string, error) {
	out := map[ids.UUID]string{}
	for _, id := range want {
		if label, ok := s[entityType][id]; ok {
			out[id] = label
		}
	}
	return out, nil
}

func TestAStageOrMemberIsNamedOnlyWhereTheReaderMayReadIt(t *testing.T) {
	qualify, propose := ids.NewV7(), ids.NewV7()
	ann, hidden := ids.NewV7(), ids.NewV7()
	svc := NewService(nil, nil, func() time.Time { return clocktest.Now(t) }).WithReferenceNames(stubNames{
		"stage": {qualify: "Qualify", propose: "Propose"},
		"user":  {ann: "Ann Lee"},
	})
	changes := changesOf(dealRow(
		`{"stage_id": "`+qualify.String()+`", "owner_id": "`+hidden.String()+`"}`,
		`{"stage_id": "`+propose.String()+`", "owner_id": "`+ann.String()+`"}`))
	if err := svc.labelReferences(context.Background(), [][]crmcontracts.MagicFieldChange{changes}); err != nil {
		t.Fatal(err)
	}
	stage := changeOf(t, changes, "stage_id")
	if stage.BeforeLabel == nil || *stage.BeforeLabel != "Qualify" || stage.AfterLabel == nil || *stage.AfterLabel != "Propose" {
		t.Errorf("stage labels %v → %v, want Qualify → Propose", stage.BeforeLabel, stage.AfterLabel)
	}
	owner := changeOf(t, changes, "owner_id")
	if owner.BeforeLabel != nil || owner.AfterLabel == nil || *owner.AfterLabel != "Ann Lee" {
		t.Errorf("owner labels %v → %v, want no name for the unreadable seat and Ann Lee after", owner.BeforeLabel, owner.AfterLabel)
	}
}
