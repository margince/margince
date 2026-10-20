// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"errors"
	"net/http"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestRefusedFieldNamesTheFieldAndKeepsTheHelpersSentence(t *testing.T) {
	err := refusedField("scope", invalid("an owner scope names which owner"))
	fault, ok := httperr.Classify(err)
	if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 {
		t.Fatalf("want one 422 field, got %+v", fault)
	}
	if got := fault.Fields[0]; got.Field != "scope" || got.Message != "an owner scope names which owner" {
		t.Fatalf("field refusal lost its sentence: %+v", got)
	}
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatal("a refusal must still unwrap to the invalid-argument sentinel")
	}
}

func TestRefusedFieldLeavesOtherFailuresAlone(t *testing.T) {
	if got := refusedField("scope", apperrors.ErrNotFound); !errors.Is(got, apperrors.ErrNotFound) {
		t.Fatalf("a missing row became %v", got)
	}
	named := refuse("value", "out_of_range", "enter a non-negative target")
	if got := refusedField("scope", named); got != named {
		t.Fatalf("an already named refusal was renamed: %v", got)
	}
}

func TestAnIdentityChangeNamesTheFieldThatDiffers(t *testing.T) {
	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	window := crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 1, 0)}
	before := crmcontracts.ReportingTarget{
		Definition: crmcontracts.ReportingTargetInput{Metric: "bookings_won", PeriodKind: "month"},
		Interval:   window,
		Unit:       "EUR",
	}
	quarter := crmcontracts.ReportingWindow{StartAt: start, EndAt: start.AddDate(0, 3, 0)}
	nextMonth := crmcontracts.ReportingWindow{StartAt: start.AddDate(0, 1, 0), EndAt: start.AddDate(0, 2, 0)}
	cases := []struct {
		name     string
		change   func(*crmcontracts.ReportingTargetInput)
		interval crmcontracts.ReportingWindow
		unit     string
		field    string
	}{
		{"metric", func(in *crmcontracts.ReportingTargetInput) { in.Metric = "other" }, window, "EUR", "metric"},
		{"unit follows the metric", func(*crmcontracts.ReportingTargetInput) {}, window, "count", "metric"},
		{"scope", func(in *crmcontracts.ReportingTargetInput) { in.Scope.Kind = "team" }, window, "EUR", "scope"},
		{"period kind with the same start", func(in *crmcontracts.ReportingTargetInput) { in.PeriodKind = "fiscal_quarter" }, quarter, "EUR", "period_kind"},
		{"period start", func(*crmcontracts.ReportingTargetInput) {}, nextMonth, "EUR", "period_start"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			in := before.Definition
			test.change(&in)
			fault, ok := httperr.Classify(refuseIdentityChange(before, in, test.interval, test.unit))
			if !ok || len(fault.Fields) != 1 || fault.Fields[0].Field != test.field {
				t.Fatalf("want a refusal naming %q, got %+v", test.field, fault)
			}
		})
	}
	if err := refuseIdentityChange(before, before.Definition, window, "EUR"); err != nil {
		t.Fatalf("an unchanged identity was refused: %v", err)
	}
}
