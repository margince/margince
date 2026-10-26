// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An erased address can book again through the public page. A refusal there
// would tell an anonymous caller who asked to be erased, so the erased address
// gets the answer any other address gets. The erasure list keeps it.

import (
	"context"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

const (
	erasedBooker = "geloescht@visitor.example"
	cleanBooker  = "neu@visitor.example"
)

func TestAnErasedAddressBooksAndIsAnsweredAsACleanOne(t *testing.T) {
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	transactional := seededTransactionalPurposeID(t, e)
	monday := nextMonday()
	book := func(email string, at time.Duration) (int, map[string]any) {
		var out map[string]any
		status := publicCall(t, e, "POST", base, AnyMap{
			"start": monday.Add(at), "end": monday.Add(at + 30*time.Minute),
			"booker": AnyMap{"name": "Wieder Da", "email": email},
			"consent": AnyMap{
				"purpose_id": transactional, "policy_version": "pp-2026-01",
				"wording": "You agree we may contact you about this meeting.",
			},
		}, nil, &out)
		return status, out
	}

	if status, _ := book(erasedBooker, time.Hour); status != http.StatusCreated {
		t.Fatalf("the first booking → %d, want 201", status)
	}
	eraseBooker(t, e, contactIDByEmail(t, e, erasedBooker))

	erasedStatus, erasedAnswer := book(erasedBooker, 2*time.Hour)
	cleanStatus, cleanAnswer := book(cleanBooker, 3*time.Hour)
	if erasedStatus != http.StatusCreated || cleanStatus != http.StatusCreated {
		t.Fatalf("the erased address → %d, a clean one → %d; want 201 for both", erasedStatus, cleanStatus)
	}
	if !reflect.DeepEqual(answerShape(erasedAnswer), answerShape(cleanAnswer)) ||
		erasedAnswer["booking"] != cleanAnswer["booking"] || erasedAnswer["marketing"] != cleanAnswer["marketing"] {
		t.Fatalf("the erased answer %v differs from the clean answer %v", erasedAnswer, cleanAnswer)
	}
	var kept int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM erasure_suppression WHERE kind = 'email' AND value_hash = $1`,
		storekit.SuppressionHash(erasedBooker)).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept != 1 {
		t.Fatalf("the booking lifted the suppression: %d row(s) left, want 1", kept)
	}
}

// eraseBooker erases the booker the way an admin fulfils an Art. 17 request.
func eraseBooker(t *testing.T, e *apptest.AppEnv, contactID string) {
	t.Helper()
	var dsr struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/data-subject-requests", AnyMap{
		"kind": "erasure", "subject_ref": contactID,
	}, nil, &dsr); status != http.StatusCreated {
		t.Fatalf("filing the erasure request → %d", status)
	}
	if status := e.Call(t, "PATCH", "/v1/data-subject-requests/"+dsr.ID, AnyMap{
		"status": "fulfilled", "resolution": "erased per Art. 17", "identity_check": "id_document",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("fulfilling the erasure request → %d", status)
	}
}

// answerShape is an answer with every value replaced by its JSON type. Two
// bookings of different slots then compare on what the caller can see.
//
//craft:ignore naked-any a decoded JSON body has no static type to name
func answerShape(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		shape := make(map[string]any, len(typed))
		for key, inner := range typed {
			shape[key] = answerShape(inner)
		}
		return shape
	case []any:
		shape := make([]any, len(typed))
		for i, inner := range typed {
			shape[i] = answerShape(inner)
		}
		return shape
	default:
		return reflect.TypeOf(value)
	}
}
