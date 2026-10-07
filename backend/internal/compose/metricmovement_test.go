// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMovementRefusesUnboundedOrUnauthorizedContextsBeforeReadingCaptures(t *testing.T) {
	evaluator := metricEvaluator{}
	permitted := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"forecast": {Read: true}}}})
	for _, ctx := range []context.Context{context.Background(), permitted} {
		chart, facts, err := evaluator.movementChart(ctx, nil, crmcontracts.ReportingContext{}, crmcontracts.ReportingChart{})
		if err != nil || chart.Coverage.Status != "unsupported" || len(facts) != 0 {
			t.Fatalf("unsupported movement: %+v %+v %v", chart, facts, err)
		}
	}
}

func TestMovementKeepsUnpricedContributionsExplicitAndOmitsZeroBuckets(t *testing.T) {
	at := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	opening := reportingCapture{id: ids.NewV7(), at: at}
	closing := reportingCapture{id: ids.NewV7(), at: at.AddDate(0, 0, 7)}
	movement := forecasting.Movement{OpeningMinor: 100, ClosingMinor: 125, Buckets: []forecasting.Bucket{{Name: "amount", AmountMinor: 25}, {Name: "won", AmountMinor: 0}}}
	chart, err := projectMovement(crmcontracts.ReportingChart{}, movement, []reporting.Fact{{Money: nil}}, opening, closing)
	if err != nil {
		t.Fatal(err)
	}
	if chart.Coverage.Status != "partial" || chart.Coverage.Reason == nil || len(chart.Points) != 1 || chart.Points[0].Key != "amount" || *chart.Opening+*chart.Points[0].Value != *chart.Closing {
		t.Fatalf("misleading movement: %+v", chart)
	}
	if chart.OpeningSnapshotId == nil || ids.UUID(*chart.OpeningSnapshotId) != opening.id || chart.SnapshotId == nil || ids.UUID(*chart.SnapshotId) != closing.id {
		t.Fatal("movement lost its capture boundaries")
	}
}

func TestMovementRejectsInexactTotalsAndUntraceableSources(t *testing.T) {
	oversized := int64(reportingExactInteger + 1)
	for _, movement := range []forecasting.Movement{{OpeningMinor: oversized}, {ClosingMinor: -oversized}, {Buckets: []forecasting.Bucket{{Name: "amount", AmountMinor: oversized}}}} {
		if _, err := projectMovement(crmcontracts.ReportingChart{}, movement, nil, reportingCapture{}, reportingCapture{}); err == nil {
			t.Fatalf("inexact movement accepted: %+v", movement)
		}
	}
	for _, contribution := range []forecasting.Contribution{{DealID: "invalid", InOpen: true}, {DealID: ids.NewV7().String(), Owner: "invalid", InOpen: true}} {
		if _, err := movementAnchorFacts([]forecasting.Contribution{contribution}, "movement_opening", time.Time{}); err == nil {
			t.Fatalf("untraceable source accepted: %+v", contribution)
		}
	}
	facts, err := movementAnchorFacts([]forecasting.Contribution{{DealID: ids.NewV7().String(), InOpen: false}}, "movement_opening", time.Time{})
	if err != nil || len(facts) != 0 {
		t.Fatalf("closed deal entered open-pipeline anchor: %+v %v", facts, err)
	}
	if _, err := movementDeltaFacts([]forecasting.DealDelta{{DealID: ids.NewV7().String()}}, nil, nil, time.Time{}); err == nil {
		t.Fatal("delta without a captured source was accepted")
	}
}
