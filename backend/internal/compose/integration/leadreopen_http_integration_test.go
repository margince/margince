// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The reopen over HTTP: the way back from a disqualification, end to end.
//
// The store's own suite proves what the verb restores and where it reads that
// from. What only this reaches is the door — the route, the handler and the
// refusal a caller actually receives — and a 409 that arrived as a 500 would
// tell a screen to say "something went wrong" about a state it can explain.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type leadWire struct {
	ID                 string  `json:"id"`
	Status             string  `json:"status"`
	ArchivedAt         *string `json:"archived_at"`
	DisqualifyNote     *string `json:"disqualify_note"`
	DisqualifyReasonID *string `json:"disqualify_reason_id"`
}

func TestReopeningADisqualifiedLeadOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var lead leadWire
	if status := e.Call(t, "POST", "/v1/leads", map[string]any{
		"full_name": "Robin Reopen", "email": "robin@reopen.test", "source": "manual",
	}, nil, &lead); status != http.StatusCreated {
		t.Fatalf("create lead -> %d, want 201", status)
	}
	base := "/v1/leads/" + lead.ID

	note := "budget gone for this quarter"
	var closed leadWire
	if status := e.Call(t, "DELETE", base, map[string]any{"note": note}, nil, &closed); status != http.StatusOK {
		t.Fatalf("disqualify -> %d, want 200", status)
	}
	if closed.Status != "disqualified" || closed.ArchivedAt == nil {
		t.Fatalf("after disqualify = %+v, want disqualified and archived", closed)
	}

	var reopened leadWire
	if status := e.Call(t, "POST", base+"/reopen", nil, nil, &reopened); status != http.StatusOK {
		t.Fatalf("reopen -> %d, want 200", status)
	}
	// `new`, because that is where this lead was when it was closed — nothing
	// had moved it up the ladder. The point is that the status is READ rather
	// than assumed: a fallback would have answered `engaged` here.
	if reopened.Status != "new" {
		t.Errorf("reopened status = %q, want new — the status it was closed at", reopened.Status)
	}
	if reopened.ArchivedAt != nil {
		t.Errorf("archived_at = %v, want cleared", reopened.ArchivedAt)
	}
	if reopened.DisqualifyNote != nil || reopened.DisqualifyReasonID != nil {
		t.Errorf("the closure survived the reopen: note=%v reason=%v",
			reopened.DisqualifyNote, reopened.DisqualifyReasonID)
	}

	// The lead is on the open ladder again, which is what every list and filter
	// reads. A GET that still answered 404 would mean the archive had not lifted
	// where it counts.
	var live leadWire
	if status := e.Call(t, "GET", base, nil, nil, &live); status != http.StatusOK {
		t.Fatalf("read the reopened lead -> %d, want 200", status)
	}
	if live.Status != "new" {
		t.Errorf("live read status = %q, want new", live.Status)
	}

	// Reopening an open lead is a 409 and not a 500: the state is one the
	// screen can explain, and a caller told "something went wrong" cannot.
	if status := e.Call(t, "POST", base+"/reopen", nil, nil, nil); status != http.StatusConflict {
		t.Errorf("second reopen -> %d, want 409", status)
	}
}
