// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Placing or lifting a legal hold announces itself as the record's own
// <record>.updated event, staged beside the audit row that carries the reason.
// Driven through compose.NewLegalHoldSeam, the door the HTTP handler and the
// agent tool both reach, for each of the five holdable record types.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const holdReason = "Anwaltsschreiben 2026-14"

func TestPlacingAndLiftingALegalHoldEmitsTheRecordsUpdatedEvent(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Disputed GmbH", nil)
	leadName := "Held Lead"
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &leadName, Source: "manual"})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	project, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "Disputed rollout", CompanyID: companyIDOf(company), Source: "manual",
	})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	records := map[string]ids.UUID{
		"contact": e.SeedContact(t, "Held Contact", nil),
		"company": company,
		"lead":    ids.UUID(lead.Id),
		"deal":    e.SeedOpenDealOn(t, "Disputed deal", company),
		"project": ids.UUID(project.Id),
	}

	seam := compose.NewLegalHoldSeam(e.Pool)
	for entityType, id := range records {
		t.Run(entityType, func(t *testing.T) {
			if err := seam.SetLegalHold(e.Admin(), entityType, id, true, holdReason); err != nil {
				t.Fatalf("place: %v", err)
			}
			assertHoldEvent(t, e, entityType, id, "place_legal_hold", true)
			if err := seam.SetLegalHold(e.Admin(), entityType, id, false, holdReason); err != nil {
				t.Fatalf("lift: %v", err)
			}
			assertHoldEvent(t, e, entityType, id, "lift_legal_hold", false)
		})
	}
}

// assertHoldEvent finds the one <entityType>.updated outbox row whose trace
// names the hold's audit row, and reads the flag it announces. The trace link
// is the write shape's proof that both rows came from one transaction.
func assertHoldEvent(t *testing.T, e *Env, entityType string, id ids.UUID, action string, held bool) {
	t.Helper()
	const traced = `
		  FROM audit_log a
		  JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id' = a.id::text
		 WHERE a.entity_type = $1 AND a.entity_id = $2 AND a.action = $3`
	if n := e.WsCount(t, `SELECT count(*)`+traced, entityType, id, action); n != 1 {
		t.Fatalf("%s: %d outbox rows trace to the %s audit row, want 1", entityType, n, action)
	}
	raw := e.WsScalar(t, `SELECT o.envelope::text`+traced, entityType, id, action)
	if strings.Contains(raw, holdReason) {
		t.Errorf("%s: the event carries the hold's reason, which belongs on the audit row alone: %s", entityType, raw)
	}
	var envelope struct {
		Type   string `json:"type"`
		Entity struct {
			Type string   `json:"type"`
			ID   ids.UUID `json:"id"`
		} `json:"entity"`
		Payload struct {
			ChangedFields map[string]any `json:"changed_fields"`
		} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		t.Fatal(err)
	}
	if want := entityType + ".updated"; envelope.Type != want {
		t.Errorf("%s: event type %q, want %q", entityType, envelope.Type, want)
	}
	if envelope.Entity.Type != entityType || envelope.Entity.ID != id {
		t.Errorf("%s: event names %s:%s, want %s:%s", entityType, envelope.Entity.Type, envelope.Entity.ID, entityType, id)
	}
	got := envelope.Payload.ChangedFields
	if len(got) != 1 || got["legal_hold"] != held {
		t.Errorf("%s: changed_fields %v, want exactly legal_hold=%v", entityType, got, held)
	}
}
