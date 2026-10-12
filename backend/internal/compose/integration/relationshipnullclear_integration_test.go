// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
)

// A null on a nullable field of a relationship clears it; a field the body
// leaves out keeps its value.
func TestRelationshipPatchClearsOnNullAndKeepsWhenOmitted(t *testing.T) {
	e := setupRelationships(t)
	type edgeWire struct {
		ID               string  `json:"id"`
		Role             *string `json:"role"`
		StartedAt        *string `json:"started_at"`
		EndedAt          *string `json:"ended_at"`
		StartedPrecision *string `json:"started_precision"`
	}
	var edge edgeWire
	if status := e.Call(t, "POST", "/v1/relationships", AnyMap{
		"kind": "employment", "contact_id": e.contactID, "company_id": e.companyID,
		"role": "cto", "source": "manual", "started_at": "2024-03-01", "ended_at": "2025-03-01",
		"started_precision": "month", "employment_status": "former",
	}, nil, &edge); status != http.StatusCreated {
		t.Fatalf("create employment → %d", status)
	}
	path := "/v1/relationships/" + edge.ID

	var kept edgeWire
	if status := e.Call(t, "PATCH", path, AnyMap{"is_current_primary": false}, nil, &kept); status != http.StatusOK {
		t.Fatalf("a patch naming no nullable field → %d", status)
	}
	if kept.Role == nil || kept.StartedAt == nil || kept.EndedAt == nil {
		t.Fatalf("an omitted field was cleared: %+v", kept)
	}

	var cleared edgeWire
	if status := e.Call(t, "PATCH", path, AnyMap{"role": nil, "started_at": nil, "ended_at": nil}, nil, &cleared); status != http.StatusOK {
		t.Fatalf("a patch of nulls → %d", status)
	}
	if cleared.Role != nil || cleared.StartedAt != nil || cleared.EndedAt != nil || cleared.StartedPrecision != nil {
		t.Fatalf("a null left a value behind: role=%v started=%v ended=%v precision=%v",
			cleared.Role, cleared.StartedAt, cleared.EndedAt, cleared.StartedPrecision)
	}

	for _, field := range []string{"is_current_primary", "employment_status", "started_precision"} {
		if status := e.Call(t, "PATCH", path, AnyMap{field: nil}, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("%s: null → %d, want 422", field, status)
		}
	}
}

// A billing contact's role is what the row means, so a null cannot clear it.
func TestABillingRoleCannotBeClearedByANull(t *testing.T) {
	e := setupRelationships(t)
	status, id, _ := e.billingContact(t, AnyMap{"role": "recipient"})
	if status != http.StatusCreated {
		t.Fatalf("recipient → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/relationships/"+id, AnyMap{"role": nil}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("clearing a billing role → %d, want 422", status)
	}
}

// A null inside served_segments is a refused value, never stored as an empty
// word beside the real ones.
func TestPartnerServedSegmentsRefuseANullMember(t *testing.T) {
	e := setupRelationships(t)
	status := e.Call(t, "PUT", "/v1/companies/"+e.companyID+"/partner", AnyMap{
		"partner_role": "consulting", "served_segments": []any{"fintech", nil},
	}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("served_segments with a null member → %d, want 422", status)
	}
}
