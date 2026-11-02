// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestAnUnavailableBookingCannotMakeItsDealStalled(t *testing.T) {
	e, deal, key := dealAbsenceFixture(t)
	at := time.Date(2099, 1, 15, 12, 0, 0, 0, time.UTC)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Planned review"), OccurredAt: &at, MeetingStatus: new("booked"), HostUserID: new(ids.From[ids.UserKind](e.Rep1)), ThreadKey: key, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	assertDealPlanning(t, e, deal, false)
	run := beginUnavailableThread(t, e, key)
	assertDealPlanning(t, e, deal, false)
	finishUnavailableThread(t, e, run)
	assertDealPlanning(t, e, deal, false)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(row.Id)), nil); err != nil {
		t.Fatal(err)
	}
	assertDealPlanning(t, e, deal, true)
}

func TestAnUnavailableTaskCannotBecomeAMissingNextStep(t *testing.T) {
	e, deal, key := dealAbsenceFixture(t)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "task", Subject: new("Agreed next step"), AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), ThreadKey: key, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	assertDealMissingStep(t, e, deal, false)
	run := beginUnavailableThread(t, e, key)
	assertDealMissingStep(t, e, deal, false)
	finishUnavailableThread(t, e, run)
	assertDealMissingStep(t, e, deal, false)
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(row.Id)), nil); err != nil {
		t.Fatal(err)
	}
	assertDealMissingStep(t, e, deal, true)
}

func dealAbsenceFixture(t *testing.T) (*integration.Env, ids.DealID, string) {
	t.Helper()
	e := integration.Setup(t)
	const key = "planned-work@counterparty.example"
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(seatAddress(t, e, e.Rep1), key, "Planning", ""))
	pipeline, stage, _ := integration.DealFixture(t, e)
	born := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	row, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Recorded plan", PipelineID: pipeline, StageID: stage, OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE deal SET created_at=$1 WHERE id=$2`, born, ids.UUID(row.Id))
	return e, ids.From[ids.DealKind](ids.UUID(row.Id)), threadKeyOfMessage(t, e, key)
}

func assertDealPlanning(t *testing.T, e *integration.Env, deal ids.DealID, unplanned bool) {
	t.Helper()
	var sqlStalled bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), fmt.Sprintf("SELECT %s FROM deal d WHERE d.id=@id", deals.StalledSQL("d")), pgx.NamedArgs{"id": deal.UUID}).Scan(&sqlStalled)
	}); err != nil {
		t.Fatal(err)
	}
	row, err := e.Deals.GetDeal(e.Admin(), deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if sqlStalled != unplanned || row.Stalled == nil || *row.Stalled != unplanned {
		t.Fatalf("stalled SQL=%t, wire=%v; want %t", sqlStalled, row.Stalled, unplanned)
	}
	listed := false
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := (WeeklyAttention{}).StalledDealsOf(e.Admin(), tx, []ids.UUID{e.Rep1}, 100)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.DealID == deal.UUID {
				listed = true
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if listed != unplanned {
		t.Fatalf("weekly stalled list contains deal=%t,want %t", listed, unplanned)
	}
	assertDealMissingStep(t, e, deal, unplanned)
}

func assertDealMissingStep(t *testing.T, e *integration.Env, deal ids.DealID, missing bool) {
	t.Helper()
	assertDealMissingStepAt(t, e, deal, missing, time.Date(2090, 1, 1, 12, 0, 0, 0, time.UTC))
}

func assertDealMissingStepAt(t *testing.T, e *integration.Env, deal ids.DealID, missing bool, at time.Time) {
	t.Helper()
	got, err := dealsWithNoOpenNextStep(e.Admin(), e.Pool, []ids.UUID{deal.UUID}, at)
	if err != nil {
		t.Fatal(err)
	}
	if got[deal.UUID] != missing {
		t.Fatalf("deal reported missing a next step=%t,want %t", got[deal.UUID], missing)
	}
}

func TestAnAvailableTaskWithAnUnavailableSourceIsNotAMissingNextStep(t *testing.T) {
	e, deal, key := dealAbsenceFixture(t)
	source := capturedThreadActivity(t, e, "planned-work@counterparty.example").UUID
	at := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	family, identity := provenance.WhichDealSource, "pending-plan-question"
	task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), whichDealActor), activities.LogActivityInput{Kind: "task", Subject: new("File the enquiry"), OccurredAt: &at, AssigneeID: new(ids.From[ids.UserKind](e.Rep1)), Source: family, SourceSystem: &family, SourceID: &identity, SourceActivityID: &source, Origin: activities.OriginSystemRemediation, Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	assertDealMissingStepAt(t, e, deal, false, at)
	run := beginUnavailableThread(t, e, key)
	if available := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND activity_thread_available(thread_key)`, ids.UUID(task.Id)); available != 1 {
		t.Fatal("the task's own row was not available")
	}
	assertDealMissingStepAt(t, e, deal, false, at)
	finishUnavailableThread(t, e, run)
	assertDealMissingStepAt(t, e, deal, false, at)
}
