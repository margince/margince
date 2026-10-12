// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// maskedReader reads deals with the amount withheld, which is what an
// administrator configures when a role may work a pipeline without seeing what
// it is worth.
func maskedReader(t *testing.T) imageMask {
	t.Helper()
	mask, err := newImageMask(principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman,
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeOwn,
			FieldMasks: []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}},
		},
	}))
	if err != nil {
		t.Fatalf("building the receipt's field mask: %v", err)
	}
	return mask
}

// dealUpdate is one machine update of a deal, carrying both money fields and a
// link to the company it belongs to.
func dealUpdate(before, after string) entry {
	label := "Northwind renewal"
	return entry{
		ID: ids.NewV7(), OccurredAt: time.Now(), Action: actionUpdate, EntityType: "deal",
		EntityID: ids.NewV7(), ActorType: "system", ActorID: "agent:deepread", Label: &label,
		Before: []byte(before), After: []byte(after),
	}
}

func TestAReceiptLineWithholdsWhatTheReaderMayNotSee(t *testing.T) {
	line, _, ok := lineOf(maskedReader(t), dealUpdate(
		`{"amount_minor": 1200000, "currency": "EUR", "expected_arr_minor": 400000, "stage": "qualify", "company_id": "c0ffee00-0000-4000-8000-000000000001"}`,
		`{"amount_minor": 1500000, "currency": "EUR", "expected_arr_minor": 500000, "stage": "propose", "company_id": "c0ffee00-0000-4000-8000-000000000001"}`))
	if !ok {
		t.Fatal("a deal update was not shown")
	}
	if line.Before == nil || line.After == nil {
		t.Fatal("the images went away entirely; the line should carry what the reader may see")
	}
	for _, image := range []struct {
		side   string
		fields map[string]any
	}{{"before", *line.Before}, {"after", *line.After}} {
		// The money travels as one unit: withholding the figure alone would
		// leave the ARR and the currency saying how big the deal is.
		for _, withheld := range []string{"amount_minor", "expected_arr_minor", "currency", "company_id"} {
			if _, present := image.fields[withheld]; present {
				t.Errorf("the %s image carries %q, which this reader may not see on the deal itself", image.side, withheld)
			}
		}
		if _, present := image.fields["stage"]; !present {
			t.Errorf("the %s image dropped stage, which nothing withholds from this reader", image.side)
		}
	}
}

// A reader nothing is withheld from sees the images whole, so the filter is not
// quietly emptying every receipt.
func TestAnUnmaskedReaderSeesTheWholeImage(t *testing.T) {
	line, _, ok := lineOf(unmasked(t), dealUpdate(`{"amount_minor": 1200000}`, `{"amount_minor": 1500000}`))
	if !ok {
		t.Fatal("a deal update was not shown")
	}
	if line.After == nil {
		t.Fatal("the after image went away")
	}
	if _, present := (*line.After)["amount_minor"]; !present {
		t.Error("the after image dropped amount_minor from a reader who may read it")
	}
}
