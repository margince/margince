// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package recordsources

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// olderLead plants a lead as written before the source check.
func olderLead(t *testing.T, owner *pgx.Conn, source string, system *string) ids.UUID {
	t.Helper()
	return integration.SeedBeforeSourceCheck(t, owner, "lead",
		`INSERT INTO lead (id, full_name, source, source_system, captured_by)
		 VALUES ($1, 'Older Lead', $2, $3, 'connector:gmail')`, source, system)
}

// Each older lead word lands on the channel a contact's would. The lead's
// source_system is its replay key, so the task neither fills nor replaces it.
func TestTheTaskMovesOlderLeadsOntoAChannel(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	csv, form, site := "mirror:csv:bundle", "web_form", "siteread"
	rows := []struct {
		source string
		system *string
		origin [2]string
	}{
		{"web_form", &form, [2]string{"webform", form}},
		{"siteread", &site, [2]string{"capture", site}},
		{"mirror:csv:bundle:lead:7", &csv, [2]string{"import", csv}},
		{"gmail:msg-1", nil, [2]string{"capture", ""}},
		{"siteread", nil, [2]string{"capture", ""}},
		{"inbound", nil, [2]string{"api", ""}},
		{"connector:apollo:a-1", nil, [2]string{"api", ""}},
	}
	planted := make([]ids.UUID, len(rows))
	for i, row := range rows {
		planted[i] = olderLead(t, owner, row.source, row.system)
	}

	ctx := systemCtx(e)
	task := LeadDataTask()
	if n, capped, err := task.Steps[0].Estimate(ctx, e.Pool, 3); err != nil || n != 3 || !capped {
		t.Fatalf("the lead estimate under a cap of 3 = %d, capped %v (%v), want 3 and capped", n, capped, err)
	}
	e.RunDataTask(ctx, t, task)

	for i, row := range rows {
		if got := partyOrigin(t, e, "lead", planted[i]); got != row.origin {
			t.Errorf("a lead with source %q reads %v, want %v", row.source, got, row.origin)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'lead' AND action = 'update'
		AND before ? 'source' AND evidence ? 'data_task_run'`); n != len(rows) {
		t.Errorf("%d lead audit row(s) keep an older source, want %d", n, len(rows))
	}
	if n := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'lead.updated'`); n != len(rows) {
		t.Errorf("%d lead.updated event(s), want one per corrected lead", n)
	}
	if n, _, err := task.Steps[0].Estimate(ctx, e.Pool, 10); err != nil || n != 0 {
		t.Errorf("the lead step still estimates %d row(s) (%v), want none", n, err)
	}
}

// An older lead is judged by the unvalidated check on its next write. An edit
// before the task goes through and moves the lead onto its channel.
func TestAnEditBeforeTheTaskMovesAnOlderLeadOntoItsChannel(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	form := "web_form"
	older := olderLead(t, integration.OwnerConn(t), "web_form", &form)
	name := "Renamed Lead"
	if _, err := e.Contacts.UpdateLead(e.Admin(), ids.From[ids.LeadKind](older), contacts.UpdateLeadInput{FullName: &name}); err != nil {
		t.Fatalf("editing an older lead: %v", err)
	}
	if got, want := partyOrigin(t, e, "lead", older), [2]string{"webform", form}; got != want {
		t.Errorf("the edited lead reads %v, want %v", got, want)
	}
}
