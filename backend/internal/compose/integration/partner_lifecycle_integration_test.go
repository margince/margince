// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The partner lifecycle fields over HTTP (A41/ADR-0032 + the partner-desk
// working surface): PUT /companies/{id}/partner carries the stage,
// next-step, segments, and gate metrics; the round-trip read returns
// exactly what was written; and a stage outside the closed lifecycle
// vocabulary is the seam's 422, never the DB CHECK's 500.

import (
	"net/http"
	"testing"
)

// partnerWire is the contract Partner shape as this suite reads it.
type partnerWire struct {
	CompanyID         string         `json:"company_id"`
	CertStatus        string         `json:"cert_status"`
	PartnerRole       string         `json:"partner_role"`
	RelationshipStage string         `json:"relationship_stage"`
	NextStep          *string        `json:"next_step"`
	NextStepDueAt     *string        `json:"next_step_due_at"`
	ServedSegments    []string       `json:"served_segments"`
	GateMetrics       map[string]int `json:"gate_metrics"`
	Version           int64          `json:"version"`
}

func TestPartnerLifecycleFieldsRoundTrip(t *testing.T) {
	e := setupRelationships(t)

	// Upsert with the full lifecycle block.
	var upserted partnerWire
	if status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", AnyMap{
		"partner_role":       "consulting",
		"cert_status":        "applied",
		"relationship_stage": "in_conversation",
		"next_step":          "Send the partnership one-pager",
		"next_step_due_at":   "2026-08-01",
		"served_segments":    []string{"manufacturing", "fintech"},
		"gate_metrics":       AnyMap{"certified_staff": 4, "retention_rate": 87},
	}, nil, &upserted); status != http.StatusOK {
		t.Fatalf("upsert partner with lifecycle fields → %d", status)
	}
	if upserted.RelationshipStage != "in_conversation" {
		t.Fatalf("upsert answered stage %q, want in_conversation", upserted.RelationshipStage)
	}

	// The read-back returns exactly what was written.
	var fetched partnerWire
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/partner", nil, nil, &fetched); status != http.StatusOK {
		t.Fatalf("get partner → %d", status)
	}
	if fetched.CompanyID != e.companyID || fetched.PartnerRole != "consulting" || fetched.CertStatus != "applied" {
		t.Fatalf("round-trip identity drifted: %+v", fetched)
	}
	if fetched.RelationshipStage != "in_conversation" {
		t.Fatalf("relationship_stage read back as %q, want in_conversation", fetched.RelationshipStage)
	}
	if fetched.NextStep == nil || *fetched.NextStep != "Send the partnership one-pager" {
		t.Fatalf("next_step read back as %v", fetched.NextStep)
	}
	if fetched.NextStepDueAt == nil || *fetched.NextStepDueAt != "2026-08-01" {
		t.Fatalf("next_step_due_at read back as %v, want the 2026-08-01 date", fetched.NextStepDueAt)
	}
	if len(fetched.ServedSegments) != 2 || fetched.ServedSegments[0] != "manufacturing" || fetched.ServedSegments[1] != "fintech" {
		t.Fatalf("served_segments read back as %v", fetched.ServedSegments)
	}
	if fetched.GateMetrics["certified_staff"] != 4 || fetched.GateMetrics["retention_rate"] != 87 {
		t.Fatalf("gate_metrics read back as %v, want certified_staff 4 / retention_rate 87", fetched.GateMetrics)
	}

	// A stage outside the closed lifecycle vocabulary is refused at the
	// seam — 422, and the stored stage stands.
	if status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", AnyMap{
		"partner_role":       "consulting",
		"relationship_stage": "best_friends",
	}, map[string]string{"If-Match": "1"}, nil); status != 422 {
		t.Fatalf("unknown relationship_stage → %d, want 422", status)
	}
	var after partnerWire
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/partner", nil, nil, &after); status != http.StatusOK {
		t.Fatalf("get partner after refusal → %d", status)
	}
	if after.RelationshipStage != "in_conversation" {
		t.Fatalf("a refused stage write landed anyway: %q", after.RelationshipStage)
	}
}

// A gate metric the column cannot hold is refused, not wrapped.
//
// JSON numbers decode to float64 and the columns are smallint, so a plain cast
// wrapped. 1e30 landed as -1, which reads as a partner failing a gate it was
// never measured against. A negative seat count stored as sent.
//
// The handler refuses all five of these. The column's own CHECK catches four
// of them, for any writer that bypasses the handler.
//
// 2.5 is the one the column cannot see. It truncates to a 2 the column accepts.
func TestPartnerGateMetricsRefuseWhatTheColumnCannotHold(t *testing.T) {
	e := setupRelationships(t)

	// A programme that fits: the baseline every refusal below is measured
	// against, so a test that refuses everything cannot pass.
	var ok partnerWire
	if status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", AnyMap{
		"partner_role": "consulting", "cert_status": "applied",
		"gate_metrics": AnyMap{"certified_staff": 4, "retention_rate": 87},
	}, nil, &ok); status != http.StatusOK {
		t.Fatalf("a programme inside the bounds → %d, want 200", status)
	}

	for _, tc := range []struct {
		name    string
		metrics AnyMap
	}{
		{"a negative seat count", AnyMap{"certified_staff": -5}},
		{"a seat count past the column", AnyMap{"certified_staff": 1e30}},
		{"a fractional seat count", AnyMap{"certified_staff": 2.5}},
		{"a retention rate above a hundred", AnyMap{"retention_rate": 150}},
		{"a negative retention rate", AnyMap{"retention_rate": -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var ignored map[string]any
			status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", AnyMap{
				"partner_role": "consulting", "cert_status": "applied",
				"gate_metrics": tc.metrics,
			}, nil, &ignored)
			if status != http.StatusUnprocessableEntity {
				t.Errorf("%v → %d, want 422", tc.metrics, status)
			}
		})
	}

	// The refusals left the programme that fits standing.
	var after partnerWire
	if status := e.Call(t, "GET", "/v1/companies/"+e.companyID+"/partner", nil, nil, &after); status != http.StatusOK {
		t.Fatalf("read the partner back → %d, want 200", status)
	}
	if after.GateMetrics["certified_staff"] != 4 || after.GateMetrics["retention_rate"] != 87 {
		t.Errorf("gate_metrics read back as %v, want the 4 / 87 the refusals should not have touched",
			after.GateMetrics)
	}
}
