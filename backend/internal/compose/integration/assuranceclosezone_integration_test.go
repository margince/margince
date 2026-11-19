// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A close date has passed when its day is over where the installation works.
// At 18:00 UTC it is already the next day in Saigon. A deal closing on the UTC
// day is late there, though not yet in UTC.
func TestTheInputCheckReadsAPastCloseInTheInstallationZone(t *testing.T) {
	e := Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"UTC"'::jsonb WHERE key = 'installation.timezone'`)
	pipeline, open, _ := DealFixture(t, e)
	now := clocktest.Now(t).UTC()
	// Tomorrow in UTC, so no midnight between the clock read and the create
	// can make the date past.
	closes := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	asOf := closes.Add(18 * time.Hour)
	owner := ids.From[ids.UserKind](e.Rep1)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Closes on a UTC day", PipelineID: pipeline, StageID: open, OwnerID: &owner, ExpectedClose: &closes,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The create stays in UTC so its own past-date rule never reads the hour.
	// The check then judges the same date in Saigon.
	e.WsExec(t, `UPDATE setting SET value = '"Asia/Ho_Chi_Minh"'::jsonb WHERE key = 'installation.timezone'`)

	scanner := assurance.NewScanner(assurance.NewStore(compose.InstallationDB(e.Pool)),
		compose.AssuranceSubjects, compose.AssuranceCoverage, compose.AssuranceConfig())
	if _, err := scanner.Scan(e.Admin(), asOf, nil); err != nil {
		t.Fatalf("running the check: %v", err)
	}
	var raised int
	if err := OwnerConn(t).QueryRow(context.Background(), `SELECT count(*) FROM assurance_exception
		WHERE subject_id = $1 AND type = $2 AND status = 'open'`, deal.Id, assurance.TypeClosePast).Scan(&raised); err != nil {
		t.Fatal(err)
	}
	if raised != 1 {
		t.Fatalf("%d open close_past findings, want 1: the date was judged in UTC, where its day is not over", raised)
	}
}
