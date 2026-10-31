// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The ranked sweep and the tail judge quiet at the read's own instant, not the
// database's. A deal at the threshold is not quiet, and one that crosses it
// after that instant is not quiet yet, in both.
func TestTheSweepAndTheTailJudgeQuietAtOneInstant(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	asOf := clocktest.Now(t).Add(-2 * time.Hour).Truncate(time.Microsecond)
	threshold := asOf.AddDate(0, 0, -deals.QuietThresholdDays)
	idle := map[string]time.Time{
		"quiet":    threshold.Add(-time.Minute),
		"exact":    threshold,
		"crossing": threshold.Add(time.Hour),
		"recent":   asOf.AddDate(0, 0, -1),
	}
	byID := map[ids.UUID]string{}
	for name, at := range idle {
		id := e.SeedDeal(t, name, pipeline, stage, &e.Rep1)
		e.WsExec(t, `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, id, at)
		byID[id] = name
	}
	owners := &storekit.OwnerSet{IDs: []ids.UUID{e.Rep1}}
	swept, _, err := quietDealScanWithClock(e.Pool, deals.QuietThresholdDays, func() time.Time { return asOf })(e.Admin(), owners)
	if err != nil {
		t.Fatal(err)
	}
	tailed, _, _, err := quietDealTail(e.Pool, deals.QuietThresholdDays)(e.Admin(), asOf, owners, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for read, rows := range map[string][]ids.UUID{"sweep": dealIDsOf(swept), "tail": dealIDsOf(tailed)} {
		if len(rows) != 1 || byID[rows[0]] != "quiet" {
			names := []string{}
			for _, id := range rows {
				names = append(names, byID[id])
			}
			t.Errorf("the %s judged %v quiet, want only the deal idle past the threshold at the read's instant", read, names)
		}
	}
}

func dealIDsOf(candidates []agents.SlippingDeal) []ids.UUID {
	out := make([]ids.UUID, len(candidates))
	for i, deal := range candidates {
		out[i] = deal.DealID
	}
	return out
}

// A deal past the sweep whose brief row stands on the ranked page is that one
// row. The tail never serves it as at risk. Done on it as an at-risk row
// answers not found.
func TestDoneRefusesADealTheBriefRowReportsAbove(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	at := clocktest.Now(t).Truncate(time.Microsecond)
	clock := func() time.Time { return at }
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	today := installationDay(t, e, at)
	for i := range slippingScanLimit + 1 {
		id := e.SeedDeal(t, fmt.Sprintf("Swept %d", i), pipeline, stage, &e.Rep1)
		e.WsExec(t, `UPDATE deal SET expected_close_date = $2 WHERE id = $1`, id, today.AddDate(0, 0, -(10+i)).Format(time.DateOnly))
	}
	owner := ids.From[ids.UserKind](e.Rep1)
	amount, currency, closes := int64(90000000), "EUR", today.AddDate(0, 0, 3)
	row, err := e.Deals.WithClock(clock).CreateDeal(ctx, deals.CreateDealInput{
		Name: "Briefed past the sweep", PipelineID: pipeline, StageID: stage, OwnerID: &owner,
		AmountMinor: &amount, Currency: &currency, ExpectedClose: &closes, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	briefed := ids.UUID(row.Id)
	if _, err := briefs.NewBriefEngine(e.Pool, e.Contacts).SnapshotRun(ctx, at); err != nil {
		t.Fatal(err)
	}
	// Past its close now, so it is at risk and sorts after every swept deal.
	e.WsExec(t, `UPDATE deal SET expected_close_date = $2 WHERE id = $1`, briefed, today.AddDate(0, 0, -1).Format(time.DateOnly))
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), clock)
	asBrief := false
	for cursor := ""; ; {
		page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 100, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Queue {
			if item.Subject != nil && ids.UUID(item.Subject.Id) == briefed {
				if item.Source == crmcontracts.WorklistItemSourceDealAtRisk {
					t.Fatal("the briefed deal was served as at risk beside its brief row")
				}
				asBrief = asBrief || item.Source == crmcontracts.WorklistItemSourceBriefItem
			}
		}
		if page.NextCursor == nil {
			break
		}
		cursor = *page.NextCursor
	}
	if !asBrief {
		t.Fatal("the deal has no brief row on the page, so this proves nothing")
	}
	done := []attention.RowRef{{Source: string(crmcontracts.WorklistItemSourceDealAtRisk), RowID: briefed.String()}}
	if err := svc.MarkRowsDone(ctx, done); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("Done on the deal as an at-risk row answered %v, want not found", err)
	}
}
