// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"
	"time"
)

// The 201 for a scheduling and a later read of the same row agree on when the
// row was written. A client renders the 201 directly, so a zero time there shows
// a message created in the year 1.
func TestASchedulingAnswersWithTheStampsTheRowHolds(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	type stamps struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	var answered stamps
	status := p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{
		"subject": "Monday morning", "body": "Written the night before.",
		"to": []string{"buyer@preflight.test"}, "consent_purpose": "transactional",
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}, nil, &answered)
	if status != http.StatusCreated {
		t.Fatalf("scheduling a send → %d, want 201", status)
	}

	var read stamps
	if code := p.Call(t, "GET", "/v1/scheduled-sends/"+answered.ID, nil, nil, &read); code != http.StatusOK {
		t.Fatalf("reading the scheduled send → %d", code)
	}
	if answered.CreatedAt.IsZero() || !answered.CreatedAt.Equal(read.CreatedAt) {
		t.Fatalf("the 201 says created_at %s, the read says %s; both must name the insert", answered.CreatedAt, read.CreatedAt)
	}
	if answered.UpdatedAt.IsZero() || !answered.UpdatedAt.Equal(read.UpdatedAt) {
		t.Fatalf("the 201 says updated_at %s, the read says %s; both must name the insert", answered.UpdatedAt, read.UpdatedAt)
	}
}
