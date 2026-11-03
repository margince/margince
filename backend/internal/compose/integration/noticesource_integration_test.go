// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestUnknownAcquisitionIsReviewedBeforeItCanBecomeANoticeDuty(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "Source Review Example", "source": "manual"}, nil, &contact); status != http.StatusCreated {
		t.Fatal(status)
	}
	driveNoticeCase(t, e, contact.ID)
	driveNoticeCase(t, e, contact.ID)
	var id, state, rule string
	var due *time.Time
	var count int
	if err := e.Owner.QueryRow(context.Background(), `SELECT id, state, rule, due_at FROM privacy_notice_case WHERE contact_id = @contact`, pgx.NamedArgs{"contact": contact.ID}).Scan(&id, &state, &rule, &due); err != nil {
		t.Fatal(err)
	}
	if state != "source_review" || rule != "unassessed" || due != nil {
		t.Fatalf("%s %s %v", state, rule, due)
	}
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM privacy_notice_case WHERE contact_id = @contact`, pgx.NamedArgs{"contact": contact.ID}).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	path := "/v1/privacy/notice-case-reviews/" + id + "/source"
	if status := e.Call(t, "GET", "/v1/privacy/notice-cases/"+id, nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("legacy duty endpoint exposed source review: %d", status)
	}
	bearer := apptest.PassportBearer(t, e, "source-review-probe", "read", "write")
	if status := e.Call(t, "POST", path, AnyMap{"kind": "subject_initiated", "note": "Agent assessment"}, bearer, nil); status != http.StatusForbidden {
		t.Fatalf("agent assessment -> %d", status)
	}

	obtained := time.Date(2020, 1, 31, 10, 0, 0, 0, time.UTC)
	var assessed struct {
		State, Rule string
		DueAt       *time.Time `json:"due_at"`
		SourceKind  *string    `json:"source_kind"`
	}
	if status := e.Call(t, "POST", path, AnyMap{"kind": "referral", "note": "Confirmed referral", "obtained_at": obtained.Format(time.RFC3339)}, nil, &assessed); status != http.StatusOK {
		t.Fatalf("assessment -> %d", status)
	}
	expected := time.Date(2020, 2, 29, 10, 0, 0, 0, time.UTC)
	if assessed.State != "open" || assessed.Rule != "art14" || assessed.DueAt == nil || !assessed.DueAt.Equal(expected) || assessed.SourceKind == nil || *assessed.SourceKind != "referral" {
		t.Fatalf("assessment = %+v", assessed)
	}
	if status := e.Call(t, "POST", path, AnyMap{"kind": "subject_initiated", "note": "Overwrite"}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("second assessment -> %d", status)
	}
}

func TestDirectSourceAssessmentEndsReviewWithoutInventingADisclosure(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "Direct Source Example", "source": "manual"}, nil, &contact); status != http.StatusCreated {
		t.Fatal(status)
	}
	driveNoticeCase(t, e, contact.ID)
	var id string
	if err := e.Owner.QueryRow(context.Background(), `SELECT id FROM privacy_notice_case WHERE contact_id = @contact`, pgx.NamedArgs{"contact": contact.ID}).Scan(&id); err != nil {
		t.Fatal(err)
	}
	var assessed struct {
		State string
		DueAt *time.Time `json:"due_at"`
		Note  string     `json:"resolution_note"`
	}
	if status := e.Call(t, "POST", "/v1/privacy/notice-case-reviews/"+id+"/source", AnyMap{"kind": "subject_initiated", "note": "Original incoming message confirms direct collection"}, nil, &assessed); status != http.StatusOK {
		t.Fatal(status)
	}
	if assessed.State != "exempt_with_reason" || assessed.DueAt != nil || assessed.Note == "" {
		t.Fatalf("assessment = %+v", assessed)
	}
}
