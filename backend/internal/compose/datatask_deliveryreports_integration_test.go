// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type reportFixture struct {
	e      *integration.Env
	ctx    context.Context
	system context.Context
}

func newReportFixture(t *testing.T) reportFixture {
	t.Helper()
	e := integration.Setup(t)
	sys := principal.WithWorkspaceID(context.Background(), e.WS)
	sys = principal.WithCorrelationID(sys, ids.NewV7())
	sys = principal.WithActor(sys, principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	return reportFixture{e: e, ctx: e.As(e.AdminUser, nil, importerPerms()), system: sys}
}

func (f reportFixture) lead(t *testing.T, name string) ids.UUID {
	t.Helper()
	lead := ids.NewV7()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
		 VALUES ($1, $2, 'new', 'inbound', 'human:x', $3)`, lead, name, f.e.Rep1); err != nil {
		t.Fatal(err)
	}
	if err := contacts.NewStore(f.e.DB()).RecomputeLeadScore(f.system, ids.From[ids.LeadKind](lead), clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}
	return lead
}

// file writes one imported received mail through the real activities writer
// and runs the lead workflows the outbox would run for it. A sender the write
// door now refuses is set on the writer's input after the mapper. That is how
// such a row got stored before the door refused it.
func (f reportFixture) file(t *testing.T, lead ids.UUID, sourceID, from string, sourceSystem *string) ids.UUID {
	t.Helper()
	req := crmcontracts.CreateActivityRequest{
		Kind: "email", Source: "manual", SourceSystem: sourceSystem, SourceId: &sourceID,
		Direction: directionPtr("inbound"), Subject: &sourceID,
		Links: &[]struct {
			EntityId   openapi_types.UUID                                `json:"entity_id"` //nolint:staticcheck // mirrors the generated inline struct, whose field is spelled EntityId
			EntityType crmcontracts.CreateActivityRequestLinksEntityType `json:"entity_type"`
		}{{EntityId: openapi_types.UUID(lead), EntityType: crmcontracts.CreateActivityRequestLinksEntityTypeLead}},
	}
	in, err := activities.LogActivityInputFromImporter(req)
	if err != nil {
		t.Fatal(err)
	}
	in.EmailFrom = from
	in.EmailTo = []string{"us@own.test"}
	act, _, err := activities.NewStore(f.e.DB()).LogActivity(f.ctx, in)
	if err != nil {
		t.Fatalf("filing %s: %v", sourceID, err)
	}
	f.runLeadWorkflows(t, ids.UUID(act.Id))
	return ids.UUID(act.Id)
}

func participantsPtr(from, to string) *struct {
	Cc   *[]string `json:"cc,omitempty"`
	From *string   `json:"from,omitempty"`
	To   *[]string `json:"to,omitempty"`
} {
	return &struct {
		Cc   *[]string `json:"cc,omitempty"`
		From *string   `json:"from,omitempty"`
		To   *[]string `json:"to,omitempty"`
	}{From: &from, To: &[]string{to}}
}

func directionPtr(d string) *crmcontracts.CreateActivityRequestDirection {
	out := crmcontracts.CreateActivityRequestDirection(d)
	return &out
}

func (f reportFixture) runLeadWorkflows(t *testing.T, activity ids.UUID) {
	t.Helper()
	store := contacts.NewStore(f.e.DB())
	handlers := append(contacts.LeadScoreWorkflows(store), contacts.LeadSLAWorkflows(store)...)
	ev := workflow.Event{
		ID: ids.NewV7(), Type: "activity.captured", WorkspaceID: f.e.WS, OccurredAt: clocktest.Now(t),
		Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: activity},
	}
	for _, h := range handlers {
		if h.Spec().Name != "lead_status_ladder" && h.Spec().Name != "recompute_lead_score" {
			continue
		}
		eff, err := h.Plan(f.system, ev)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Apply(f.system, ev, eff, nil); err != nil {
			t.Fatalf("%s: %v", h.Spec().Name, err)
		}
	}
}

func (f reportFixture) state(t *testing.T, lead ids.UUID) (status string, score int) {
	t.Helper()
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT status, score FROM lead WHERE id = $1`, lead).Scan(&status, &score); err != nil {
		t.Fatal(err)
	}
	return status, score
}

func (f reportFixture) archived(t *testing.T, activity ids.UUID) bool {
	t.Helper()
	return f.e.WsScalar(t, `SELECT (archived_at IS NOT NULL)::text FROM activity WHERE id = $1`, activity) == "true"
}

func (f reportFixture) runTask(t *testing.T) {
	t.Helper()
	runMachineAddressTask(principal.SystemActing(f.e.Admin(), "system:datatask-test"), t, f.e,
		deliveryReportsDataTask(f.e.Pool, func() time.Time { return clocktest.Now(t) }))
}

func TestTheDeliveryReportTaskStepsAnEngagedLeadBackAndKeepsEverythingElse(t *testing.T) {
	f := newReportFixture(t)
	importer := new("mirror:hubspot")
	conn := integration.OwnerConn(t)

	steps := f.lead(t, "Bounced Lead")
	_, baseline := f.state(t, steps)
	bounce := f.file(t, steps, "bounce-1", "postmaster@client.test", importer)
	if status, score := f.state(t, steps); status != "engaged" || score <= baseline {
		t.Fatalf("fixture: the stored bounce left the lead %q at score %d (baseline %d), so it would test nothing", status, score, baseline)
	}

	byHand := f.lead(t, "Human Placed Lead")
	handBounce := f.file(t, byHand, "bounce-2", "mailer-daemon@client.test", importer)
	if _, err := conn.Exec(context.Background(), `UPDATE lead SET status = 'engaged', status_set_by = 'human' WHERE id = $1`, byHand); err != nil {
		t.Fatal(err)
	}

	answered := f.lead(t, "Answered Lead")
	answeredBounce := f.file(t, answered, "bounce-3", "bounces-12@client.test", importer)
	reply := f.file(t, answered, "reply-3", "answered@client.test", importer)

	edited := f.lead(t, "Edited Report Lead")
	editedBounce := f.file(t, edited, "bounce-4", "postmaster@edited.test", importer)
	if _, err := conn.Exec(context.Background(), `UPDATE activity SET version = 2 WHERE id = $1`, editedBounce); err != nil {
		t.Fatal(err)
	}

	foreign := f.lead(t, "Unknown Source Lead")
	foreignBounce := f.file(t, foreign, "bounce-5", "postmaster@elsewhere.test", new("hubspot"))

	malformed := f.lead(t, "Malformed Sender Lead")
	malformedMail := f.file(t, malformed, "mail-6", "not-an-address", importer)

	f.runTask(t)

	for name, id := range map[string]ids.UUID{"bounce": bounce, "human-placed bounce": handBounce, "bounce beside a reply": answeredBounce} {
		if !f.archived(t, id) {
			t.Errorf("the %s is still filed as received mail", name)
		}
	}
	if status, score := f.state(t, steps); status != "new" || score != baseline {
		t.Errorf("the stepped lead reads %q at score %d, want new at %d", status, score, baseline)
	}
	if status, _ := f.state(t, byHand); status != "engaged" {
		t.Errorf("a status a human placed became %q", status)
	}
	if f.archived(t, reply) {
		t.Error("the lead's own reply was archived")
	}
	if status, _ := f.state(t, answered); status != "engaged" {
		t.Errorf("a lead with a real reply reads %q, want engaged", status)
	}
	for name, id := range map[string]ids.UUID{"edited report": editedBounce, "unknown source": foreignBounce, "malformed sender": malformedMail} {
		if f.archived(t, id) {
			t.Errorf("the %s was archived, but it is not an unedited import from the mail system", name)
		}
	}

	step := deliveryReportsDataTask(f.e.Pool, func() time.Time { return clocktest.Now(t) }).Steps[0]
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 50, RunID: ids.NewV7()})
		if err == nil && out.Changed != 0 {
			t.Errorf("a second run changed %d row(s)", out.Changed)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTheDeliveryReportTaskNeverLowersALeadBelowAStatusAHumanPlacedBeforeTheBounce(t *testing.T) {
	f := newReportFixture(t)
	lead := f.lead(t, "Human Then Bounce Lead")
	contacted := "contacted"
	if _, err := contacts.NewStore(f.e.DB()).UpdateLead(f.ctx, ids.From[ids.LeadKind](lead), contacts.UpdateLeadInput{Status: &contacted}); err != nil {
		t.Fatalf("the human moving the lead to contacted: %v", err)
	}
	f.file(t, lead, "bounce-7", "postmaster@client.test", new("mirror:hubspot"))
	if status, _ := f.state(t, lead); status != "engaged" {
		t.Fatalf("fixture: the bounce left the lead %q, so it would test nothing", status)
	}

	f.runTask(t)

	if status, _ := f.state(t, lead); status != "contacted" {
		t.Errorf("the repaired lead reads %q, want the contacted a human placed", status)
	}
}

func TestTheDeliveryReportTaskPagesOverRowsItKeepsAndRollsBackWithItsBatch(t *testing.T) {
	f := newReportFixture(t)
	importer := new("mirror:hubspot")
	lead := f.lead(t, "Paged Lead")
	first := f.file(t, lead, "reply-a", "paged@client.test", importer)
	second := f.file(t, lead, "bounce-a", "postmaster@client.test", importer)
	step := deliveryReportsDataTask(f.e.Pool, func() time.Time { return clocktest.Now(t) }).Steps[0]
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")

	rollback := errors.New("roll back after the write")
	err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 2, RunID: ids.NewV7()})
		if err != nil {
			return err
		}
		if out.Changed != 1 {
			t.Errorf("the batch changed %d row(s), want the one bounce", out.Changed)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if f.archived(t, second) {
		t.Fatal("a rolled-back batch left its archive")
	}
	if status, _ := f.state(t, lead); status != "engaged" {
		t.Fatalf("a rolled-back batch changed the lead to %q", status)
	}

	var cursor []byte
	for range 3 {
		if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
			out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: cursor, RunID: ids.NewV7()})
			cursor = out.Cursor
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if !f.archived(t, second) || f.archived(t, first) {
		t.Errorf("after paging one row at a time: bounce archived=%v, reply archived=%v", f.archived(t, second), f.archived(t, first))
	}
}

func TestAnImportedBounceNoticeIsRefusedAtTheWriteDoorAndLeavesTheLeadAlone(t *testing.T) {
	f := newReportFixture(t)
	lead := f.lead(t, "Door Lead")
	_, baseline := f.state(t, lead)
	post := func(from, id string) (int, crmcontracts.Activity) {
		return postActivity(f.ctx, t, f.e, crmcontracts.CreateActivityRequest{
			Kind: "email", Source: "manual", SourceSystem: new("mirror:hubspot"), SourceId: &id,
			Direction:    directionPtr("inbound"),
			Participants: participantsPtr(from, "us@own.test"),
			Links: &[]struct {
				EntityId   openapi_types.UUID                                `json:"entity_id"` //nolint:staticcheck // mirrors the generated inline struct, whose field is spelled EntityId
				EntityType crmcontracts.CreateActivityRequestLinksEntityType `json:"entity_type"`
			}{{EntityId: openapi_types.UUID(lead), EntityType: crmcontracts.CreateActivityRequestLinksEntityTypeLead}},
		})
	}
	if got, _ := post("MAILER-DAEMON@client.test", "bounce-door"); got != http.StatusUnprocessableEntity {
		t.Fatalf("a bounce notice answered %d, want 422", got)
	}
	if status, score := f.state(t, lead); status != "new" || score != baseline {
		t.Fatalf("a refused bounce moved the lead to %q at %d", status, score)
	}
	got, reply := post("door@client.test", "reply-door")
	if got != http.StatusCreated {
		t.Fatalf("a reply from the lead answered %d, want 201", got)
	}
	f.runLeadWorkflows(t, ids.UUID(reply.Id))
	if status, score := f.state(t, lead); status != "engaged" || score <= baseline {
		t.Fatalf("a real reply left the lead %q at %d, want engaged above %d", status, score, baseline)
	}
}
