// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// A required body id the caller omitted must be named as a missing argument, not
// discovered as a missing row: an absent key decodes to the zero UUID with no
// error, so unguarded it reaches a lookup, matches nothing, and answers a bare
// not-found for a record the caller never mentioned.
//
// The guard is at the store entry point — the door every transport comes through —
// and it runs BEFORE any authority check or query, which is why these probes need
// no database and no actor: a store over a nil pool never reaches one.
//
// The refusal's SHAPE is proven once in platform/httperr/requirebodyid_test.go and
// asserted here through faulttest. What is left is the only question this package
// can answer: is the guard actually called for my body.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnOmittedPurposeIsNamed(t *testing.T) {
	// RecordConsentRequest.purpose_id: a consent record without a purpose is not
	// a consent record.
	store := NewStore(nil)
	ctx := context.Background()

	_, err := store.Record(ctx, RecordInput{
		PersonID: ids.New[ids.PersonKind](), NewState: "granted",
	})
	faulttest.AssertNamesOmittedID(t, err, "purpose_id")
}

// IssueDoubleOptInJSONBody.purpose_id is probed at the HANDLER, because that is
// where the operation now ends: issuance refuses outright, so there is no store
// method left to guard. The probe still has to run — a caller who omitted the
// purpose sent a malformed request whichever way the endpoint answers, and the
// refusal it gets must name the missing id rather than swallowing it behind the
// conflict.
func TestAnOmittedPurposeIsNamedOnTheIssuanceRefusal(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"purpose_id":"00000000-0000-0000-0000-000000000000"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/people/x/consent/double-opt-in", body)
	req.Header.Set("Content-Type", "application/json")

	Handlers{}.IssueDoubleOptIn(rec, req, crmcontracts.Id(ids.New[ids.PersonKind]().UUID))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("an omitted purpose is a 422 naming the field, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "purpose_id") {
		t.Errorf("the refusal must name purpose_id, got %s", rec.Body.String())
	}
}

// And the refusal itself: a well-formed request is answered with a conflict that
// explains the endpoint mints nothing, rather than a token.
func TestIssuanceRefusesAndReturnsNoToken(t *testing.T) {
	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"purpose_id":"` + ids.New[ids.PurposeKind]().String() + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/people/x/consent/double-opt-in", body)
	req.Header.Set("Content-Type", "application/json")

	Handlers{}.IssueDoubleOptIn(rec, req, crmcontracts.Id(ids.New[ids.PersonKind]().UUID))

	if rec.Code != http.StatusConflict {
		t.Fatalf("issuance refuses with a conflict, got %d: %s", rec.Code, rec.Body.String())
	}
	// The whole point: no capability leaves this endpoint.
	if strings.Contains(rec.Body.String(), "doi_") {
		t.Errorf("the refusal must carry no token material, got %s", rec.Body.String())
	}
}
