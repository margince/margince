// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The staff-mediated GDPR surface end-to-end. Workspace bootstrap seeds the
// default retention policies and no others. Fulfilling an erasure DSR runs the
// erasure, so the status flip and the deletion cannot drift apart.

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestBootstrapSeedsDefaultRetentionPolicies(t *testing.T) {
	e := setupRelationships(t)

	rows, err := e.Owner.Query(context.Background(), `
		SELECT object_type, coalesce(category, ''), retain_days, action
		FROM retention_policy ORDER BY object_type, retain_days`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var object, category, action string
		var days int
		if err := rows.Scan(&object, &category, &days, &action); err != nil {
			t.Fatal(err)
		}
		got = append(got, fmt.Sprintf("%s/%s/%d/%s", object, category, days, action))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	// The query orders by object, so the list follows the alphabet and not the
	// order the seed writes them in. No row erases a transcript or anonymizes a
	// contact: the seed destroys no record just because it is old.
	want := []string{
		"activity//1095/archive",
		"ai_call_payload/content/365/erase",
		"deal/lost/1825/archive",
		// The queue's recorded verdicts, aged with the widest window the
		// same-day next-step figure reads.
		"deal_risk_day//90/erase",
		"lead/unconverted/365/archive",
		// The provider original, aged on its own import clock.
		"raw_capture//90/erase",
	}
	if len(got) != len(want) {
		t.Fatalf("seeded %d policies %v, want the %d defaults %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("policy %d = %s, want %s (all: %v)", i, got[i], want[i], got)
		}
	}
}

func TestFulfillingAnErasureDSRExecutesTheErasure(t *testing.T) {
	e := setupRelationships(t)

	var dsr struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind": "erasure", "subject_ref": e.contactID, "due_at": "2026-08-01T00:00:00Z",
	}, nil, &dsr); status != http.StatusCreated {
		t.Fatalf("create DSR → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/data-subject-requests/"+dsr.ID, AnyMap{
		"status": "fulfilled", "resolution": "erased per Art. 17", "identity_check": "id_document",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfill DSR → %d", status)
	}

	var contact struct {
		FullName string `json:"full_name"`
	}
	if status := e.Call(t, "GET", "/v1/contacts/"+e.contactID, nil, nil, &contact); status != http.StatusOK {
		t.Fatalf("read contact → %d", status)
	}
	if contact.FullName != "Erased Subject" {
		t.Fatalf("fulfilled erasure left the subject intact: %q", contact.FullName)
	}
}
