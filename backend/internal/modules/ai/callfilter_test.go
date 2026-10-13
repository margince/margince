// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"net/http"
	"regexp"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The SQL buckets are the contract's enum, both ways: a bucket the SQL can
// return that the enum lacks would fail a client, and an enum value the SQL
// never returns is a filter that always matches nothing.
func TestTheOutcomeBucketsAreTheContractEnum(t *testing.T) {
	var buckets []string
	for _, match := range regexp.MustCompile(`(?:THEN|ELSE) '([a-z]+)'`).FindAllStringSubmatch(callOutcomeSQL(1, 2), -1) {
		buckets = append(buckets, match[1])
	}
	enum := []string{
		string(crmcontracts.AiCallOutcomeAnswered), string(crmcontracts.AiCallOutcomeFallback),
		string(crmcontracts.AiCallOutcomeDeclined), string(crmcontracts.AiCallOutcomeFailed),
	}
	if len(buckets) != len(enum) {
		t.Fatalf("the SQL names %d buckets %v, the contract %d", len(buckets), buckets, len(enum))
	}
	assertSetEqual(t, "outcome buckets", buckets, enum)
}

func TestTheCallFilterRefusesWhatWouldMatchNothing(t *testing.T) {
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	bad := crmcontracts.AiCallOutcome("retried")
	cases := map[string]func() (CallFilter, error){
		"an outcome off the enum":      func() (CallFilter, error) { return callFilterOf(nil, nil, nil, &bad, nil, nil) },
		"a window ending at its start": func() (CallFilter, error) { return callFilterOf(nil, nil, nil, nil, &from, &from) },
	}
	for name, build := range cases {
		_, err := build()
		var detailed *httperr.DetailedError
		if !errors.As(err, &detailed) || detailed.Status != http.StatusUnprocessableEntity {
			t.Errorf("%s: want a 422, got %v", name, err)
		}
	}
}

func TestTheCallFilterCarriesEveryParameter(t *testing.T) {
	from, to := time.Now().Add(-time.Hour), time.Now()
	task, model, q := "enrich", "gpt", "quota"
	outcome := crmcontracts.AiCallOutcomeFailed
	filter, err := callFilterOf(&task, &model, &q, &outcome, &from, &to)
	if err != nil {
		t.Fatal(err)
	}
	want := CallFilter{Task: task, Model: model, Query: q, Outcome: "failed", From: &from, To: &to}
	if filter != want {
		t.Errorf("filter %+v, want %+v", filter, want)
	}
}

// Placeholders follow the argument slice, and a search needle's LIKE wildcards
// match themselves.
func TestTheCallFilterBindsEachValueOnce(t *testing.T) {
	query := newCallQuery()
	where := CallFilter{Task: "enrich", Model: "m", Query: " 50%_off ", Outcome: "failed"}.where(query)
	if got, want := len(query.args), 6; got != want {
		t.Fatalf("%d arguments, want %d", got, want)
	}
	if query.args[5] != `%50\%\_off%` {
		t.Errorf("search pattern %q", query.args[5])
	}
	for n := 3; n <= 6; n++ {
		if !regexp.MustCompile(`\$` + string(rune('0'+n)) + `\b`).MatchString(where) {
			t.Errorf("placeholder $%d missing from %s", n, where)
		}
	}
}

func TestBothCallWiresCarryOutcomeAndCost(t *testing.T) {
	cost := int64(1800)
	summary := CallSummary{ID: ids.NewV7(), Task: "enrich", Outcome: "fallback", CostMicroUSD: &cost}
	list, detail := wireAiCallSummary(summary), wireAiCall(CallDetail{CallSummary: summary})
	for name, wire := range map[string]struct {
		outcome crmcontracts.AiCallOutcome
		cost    *int64
	}{"summary": {list.Outcome, list.EstimatedCostMicrousd}, "detail": {detail.Outcome, detail.EstimatedCostMicrousd}} {
		if wire.outcome != crmcontracts.AiCallOutcomeFallback || wire.cost == nil || *wire.cost != cost {
			t.Errorf("%s wire: outcome %q cost %v", name, wire.outcome, wire.cost)
		}
	}
}
