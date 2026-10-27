// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The registry half of the canonical corpus. The composed registry carries
// these tools and no direct runner reaches them. Each proof calls the tool by
// name, as an MCP client does, under a seat holding chosen grants.

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// toolRegistry is the api role's own composition over the forecast fixture.
func (e *forecastEnv) toolRegistry() *agents.Registry {
	return composedRegistryOn(InstallationDB(e.Pool))
}

// invokeAnswer invokes one tool and decodes its answer into out.
//
//craft:ignore naked-any invokeAnswer is the decode seam of these proofs; each caller passes its own answer struct
func invokeAnswer(ctx context.Context, t *testing.T, registry *agents.Registry, tool, args string, out any) {
	t.Helper()
	raw, err := registry.Invoke(ctx, tool, json.RawMessage(args))
	if err != nil {
		t.Fatalf("%s(%s) was refused: %v", tool, args, err)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatalf("%s answered %s, which is not a tool envelope: %v", tool, raw, err)
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		t.Fatalf("%s answered %s, which is not the shape this proof reads: %v", tool, envelope.Data, err)
	}
}

// daysAgo is a database timestamp a number of days back. A negative number is
// ahead. It reads the database clock because the seeded meetings do.
func (e *forecastEnv) daysAgo(t *testing.T, n int) string {
	t.Helper()
	var at string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT to_char(timezone('UTC', now() - $1 * interval '1 day'), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`, n).Scan(&at); err != nil {
		t.Fatalf("reading the database clock: %v", err)
	}
	return at
}

// seedMeetingBy writes a meeting at a given standing, credited to a host and
// linked to a lead.
func (e *forecastEnv) seedMeetingBy(t *testing.T, status string, host, lead ids.UUID) {
	t.Helper()
	meeting := e.seedID(t, `INSERT INTO activity (id, kind, direction, meeting_status, subject, occurred_at,
			thread_key, source, captured_by, host_user_id)
		VALUES ($1, 'meeting', 'outbound', $2, 'Intro call', now() - interval '3 days',
		        gen_random_uuid()::text, 'manual', 'human:x', $3)`, status, host)
	e.seedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, lead_id) VALUES ($1, $2, 'lead', $3)`,
		meeting, lead)
}

// G05: how many of my held meetings became opportunities. Held meetings only,
// credited to the asker. A prospect handed on twice counts once, and a deal at
// the same company converts nothing.
func proveG05HeldMeetingsThatConverted(t *testing.T) {
	e := setupForecast(t)
	converted := e.seedHandoffLead(t, "converted@customer.test")
	e.seedMeetingBy(t, "held", e.Rep1, converted)
	e.acceptHandoff(t, converted, e.daysAgo(t, 2))
	e.acceptHandoff(t, converted, e.daysAgo(t, 1))

	company := e.seedHandoffCompany(t)
	metOnly := e.seedHandoffLead(t, "met-only@customer.test")
	e.seedMeetingBy(t, "held", e.Rep1, metOnly)
	if _, err := e.owner.Exec(context.Background(),
		`UPDATE lead SET company_name = 'Handoff Customer' WHERE id = $1`, metOnly); err != nil {
		t.Fatalf("naming the prospect's company: %v", err)
	}
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, company_id, amount_minor, currency,
			source, captured_by)
		VALUES ($1, 'Same company', $2, $3, $4, 250000, 'EUR', 'manual', 'human:x')`,
		e.pipeline, e.stages[60], company)

	notHeld := e.seedHandoffLead(t, "booked@customer.test")
	e.seedMeetingBy(t, "booked", e.Rep1, notHeld)
	e.acceptHandoff(t, notHeld, e.daysAgo(t, 1))
	colleague := e.seedHandoffLead(t, "colleague@customer.test")
	e.seedMeetingBy(t, "held", e.Rep3, colleague)
	e.acceptHandoff(t, colleague, e.daysAgo(t, 1))

	var got struct {
		Rows []map[string]any `json:"rows"`
	}
	invokeAnswer(e.activityReader(tableLead), t, e.toolRegistry(), "run_report", `{"report":"meeting-conversion",
		"filters":{"host_user_id":"`+e.Rep1.String()+`"},"group_by":["became_opportunity"],
		"aggregates":[{"fn":"count","as":"meetings"}]}`, &got)
	if _, err := e.toolRegistry().Invoke(e.activityReader(), "run_report", json.RawMessage(`{"report":"meeting-conversion",
		"group_by":["became_opportunity"],"aggregates":[{"fn":"count","as":"meetings"}]}`)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the conversion without the lead grant answered %v, want a permission refusal", err)
	}
	byOutcome := rowsBy(t, got.Rows, "became_opportunity")
	if len(byOutcome) != 2 || byOutcome[true]["meetings"] != float64(1) || byOutcome[false]["meetings"] != float64(1) {
		t.Errorf("conversion = %v, want one converted and one not: a double handoff counted twice, "+
			"a same-company deal credited, a booked or a colleague's meeting counted", got.Rows)
	}
}

// G09: how quickly do we reply to new leads. Hours from arrival to first
// response. An imported lead is not timed, and an unanswered lead is not read
// as a zero.
func proveG09LeadFirstResponse(t *testing.T) {
	e := setupForecast(t)
	for _, hours := range []int{1, 2, 4, 8, 30} {
		e.seedLeadAnsweredAfter(t, hours, nil)
	}
	imported := "mirror:hubspot"
	e.seedLeadAnsweredAfter(t, 200, &imported)
	for range 2 {
		e.seedID(t, `INSERT INTO lead (id, full_name, source, captured_by, created_at)
			VALUES ($1, 'Waiting', 'inbound', 'human:x', now() - interval '5 days')`)
	}

	var got struct {
		Evaluation struct {
			Metrics []struct {
				ID    string   `json:"id"`
				Value *float64 `json:"value"`
			} `json:"metrics"`
		} `json:"evaluation"`
	}
	invokeAnswer(sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll), t, e.toolRegistry(), "read_reporting",
		`{"mode":"evaluate","selection":{"metrics":["lead_first_response"],"scope":{"kind":"workspace"},
		"period":"custom","interval":{"start_at":"`+e.daysAgo(t, 10)+`","end_at":"`+e.daysAgo(t, -1)+`"},
		"target_basis":"month","close_window":"all_open","blocks":["metric_reading"]}}`, &got)
	if len(got.Evaluation.Metrics) != 1 || got.Evaluation.Metrics[0].Value == nil || *got.Evaluation.Metrics[0].Value != 4 {
		t.Errorf("first response = %+v, want a median of 4 hours: the imported lead would make it 6 "+
			"and the unanswered ones read as zero would make it 3", got.Evaluation.Metrics)
	}
}

// seedLeadAnsweredAfter writes a lead that arrived five days ago and was
// answered after some hours. A source system marks it as imported.
func (e *forecastEnv) seedLeadAnsweredAfter(t *testing.T, hours int, sourceSystem *string) {
	t.Helper()
	e.seedID(t, `INSERT INTO lead (id, full_name, source, captured_by, source_system, created_at, first_response_at)
		VALUES ($1, 'Answered', 'inbound', 'human:x', $3,
		        now() - interval '5 days', now() - interval '5 days' + $2 * interval '1 hour')`,
		hours, sourceSystem)
}

// G06: which of my deals are slipping this week, and why. Each listed deal
// carries its evidence. A quiet deal with no next step says so, and a quiet
// deal with a booked meeting ahead does not. A deal touched today is absent.
func proveG06SlippingDealsWithEvidence(t *testing.T) {
	e := setupReconcile(t)
	quiet := func(name string) ids.UUID {
		deal := e.SeedDeal(t, name, e.pipeline, e.open, &e.Rep1)
		idleFor(t, e.Env, deal, 30)
		return deal
	}
	noStep := quiet("No next step")
	meetingAhead := quiet("Meeting ahead")
	e.seedMeetingHostedBy(t, meetingAhead, "Review", -48, 3600, "booked", &e.Rep1)
	fresh := e.SeedDeal(t, "Touched today", e.pipeline, e.open, &e.Rep1)

	var got struct {
		Deals []struct {
			DealID   ids.UUID `json:"deal_id"`
			Evidence []struct {
				Source string `json:"source"`
			} `json:"evidence"`
		} `json:"deals"`
	}
	invokeAnswer(e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms), t, composedRegistry(e.Env), "read_attention",
		`{"view":"slipping","quiet_days":14,"limit":10}`, &got)
	sources := map[ids.UUID][]string{}
	for _, deal := range got.Deals {
		for _, ev := range deal.Evidence {
			sources[deal.DealID] = append(sources[deal.DealID], ev.Source)
		}
		if len(deal.Evidence) == 0 {
			t.Errorf("deal %s is listed with no evidence", deal.DealID)
		}
	}
	if !slices.Contains(sources[noStep], "activity.task") {
		t.Errorf("the quiet deal with no next step has evidence %v, want the no-next-step reason", sources[noStep])
	}
	if _, listed := sources[meetingAhead]; listed {
		t.Errorf("the deal with a booked meeting ahead is listed as slipping: %v", sources[meetingAhead])
	}
	if _, listed := sources[fresh]; listed {
		t.Errorf("a deal touched today is listed as slipping: %v", sources[fresh])
	}
}

// G07: what have I promised customers that is still open. Our open promises
// are listed with due dates, and the customer's promises never are.
func proveG07OpenPromisesToCustomers(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Herr Vogt", &e.Rep1)
	seedClaim(t, e, contact, "I will send the revised offer on Friday", "commitment_ours")
	seedClaim(t, e, contact, "We will confirm the budget next week", "commitment_theirs")
	done := seedWorklistTask(t, e, contact, "Already sent")
	e.WsExec(t, `UPDATE activity SET is_done = true, done_at = now() WHERE id = $1`, done.Id.String())

	var got struct {
		Commitments []struct {
			Source string  `json:"source"`
			Quote  string  `json:"quote"`
			DueAt  *string `json:"due_at"`
		} `json:"commitments"`
	}
	invokeAnswer(e.Admin(), t, composedRegistry(e), "read_attention", `{"view":"commitments","limit":20}`, &got)
	if len(got.Commitments) != 1 || got.Commitments[0].Quote != "I will send the revised offer on Friday" {
		t.Fatalf("commitments = %+v, want only our own open promise: the customer's must never be read as ours, nor a finished task",
			got.Commitments)
	}
	if got.Commitments[0].DueAt == nil {
		t.Error("our promise carries no due date")
	}
}
