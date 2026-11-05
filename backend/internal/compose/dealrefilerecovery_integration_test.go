// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertest"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARefileRetriesCommittedActivitiesWhenItsQueueTransactionIsLost(t *testing.T) {
	f := newDealRefileFixture(t, 201)
	e := f.o.e
	integration.ApplyRiverSchema(t)
	production := newCapturedDealRefileWorker(e.Pool)
	if _, err := production.repairCohorts(principal.SystemActing(e.Admin(), dealRefileActor), []ids.UUID{f.contact}); err != nil {
		t.Fatal(err)
	}
	args := f.enqueue(t, "deal.created", f.deal)
	spec, ok := jobs.SpecFor(args.Kind())
	if !ok {
		t.Fatal("missing declaration")
	}
	worker := rivertest.NewWorker(t, riverpgxv5.New(e.Pool), &river.Config{}, jobs.Govern(production, spec))
	tx, err := e.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	result, workErr := worker.Work(t.Context(), t, tx, args, nil)
	if err := tx.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	if workErr != nil {
		t.Fatal(workErr)
	}
	if result.EventKind != river.EventKindJobSnoozed {
		t.Fatalf("first page ended as %s", result.EventKind)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE deal_id=$1`, f.deal); got != capture.DealRefileLimit {
		t.Fatalf("domain transactions committed %d links", got)
	}
	f.run(t, args)
	if got := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE deal_id=$1`, f.deal); got != 203 {
		t.Fatalf("recovery left %d links", got)
	}
	before := e.WsCount(t, `SELECT count(*) FROM audit_log`)
	f.run(t, args)
	if e.WsCount(t, `SELECT count(*) FROM audit_log`) != before {
		t.Fatal("completed replay repeated domain writes")
	}
}

func TestAContactRelinkTriggersRefileButMachineRelinksDoNot(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	integration.ApplyRiverSchema(t)
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](f.mail), activities.RelinkActivityInput{EntityType: "contact", EntityID: f.contact}); err != nil {
		t.Fatal(err)
	}
	args := f.enqueue(t, eventActivityUpdated, f.mail)
	if len(args.Scope.Activities) != 1 || args.Scope.Activities[0] != f.mail {
		t.Fatalf("relink scope=%+v", args.Scope)
	}
	for _, actor := range []string{cohortPromoteActor, dealRefileActor} {
		if err := e.DB().Tx(principal.SystemActing(e.Admin(), dealRefileActor), func(tx pgx.Tx) error {
			scope, err := dealRefileScope(e.Admin(), tx, e.Pool, events.Envelope{Type: eventActivityUpdated, Entity: events.EntityRef{Type: "activity", ID: f.mail}, Actor: events.Actor{ID: actor}})
			if !scope.Empty() {
				t.Fatalf("machine relink queued %+v", scope)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	f.run(t, args)
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, f.mail, f.deal) != 1 {
		t.Fatal("human contact relink did not reach the filing worker")
	}
}

func TestDealEditsWithoutACompanyChangeStillScheduleReconsideration(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	integration.ApplyRiverSchema(t)
	if _, err := f.o.e.Deals.UpdateDeal(f.o.e.Admin(), ids.From[ids.DealKind](f.deal), deals.UpdateDealInput{Name: new("Changed requirements")}); err != nil {
		t.Fatal(err)
	}
	args := f.enqueue(t, "deal.updated", f.deal)
	if len(args.Scope.Companies) != 1 || args.Scope.Companies[0] != f.company {
		t.Fatalf("non-company edit lost current company: %+v", args.Scope)
	}
}

func TestMalformedDealTriggersAreUnactionable(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	for _, event := range []string{"deal.updated", deals.EventDealStageChanged} {
		err := f.o.e.DB().Tx(f.o.e.Admin(), func(tx pgx.Tx) error {
			_, err := dealRefileScope(f.o.e.Admin(), tx, f.o.e.Pool, events.Envelope{Type: event, Entity: events.EntityRef{Type: "deal", ID: f.deal}, Payload: json.RawMessage(`{"broken"`)})
			return err
		})
		if !errors.Is(err, events.ErrUnactionable) {
			t.Fatalf("%s: %v", event, err)
		}
	}
}

type editedDealCandidates struct {
	capture.DealCandidates
	edit func() error
}

func (c *editedDealCandidates) OpenDealsOfCompany(ctx context.Context, tx pgx.Tx, company ids.UUID) ([]ids.UUID, error) {
	result, err := c.DealCandidates.OpenDealsOfCompany(ctx, tx, company)
	if err != nil {
		return nil, err
	}
	if c.edit != nil {
		edit := c.edit
		c.edit = nil
		if err := edit(); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func TestRefileRechecksADealEditedAfterTheCandidateRead(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	worker := newCapturedDealRefileWorker(e.Pool)
	ctx := principal.SystemActing(e.Admin(), dealRefileActor)
	if _, err := worker.repairCohorts(ctx, []ids.UUID{f.contact}); err != nil {
		t.Fatal(err)
	}
	other := ids.From[ids.CompanyKind](e.SeedCompany(t, "Other account", nil))
	worker.repair.Candidates = &editedDealCandidates{DealCandidates: worker.repair.Candidates, edit: func() error {
		_, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](f.deal), deals.UpdateDealInput{CompanyID: &other})
		return err
	}}
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, err := worker.repair.RefileTx(ctx, tx, ids.From[ids.ActivityKind](f.mail), f.o.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='deal'`, f.mail) != 0 {
		t.Fatal("stale candidate evidence filed the activity on another account")
	}
}
