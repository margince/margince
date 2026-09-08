// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What the rejection transport refuses before the store is reached.
//
// The shape checks are here rather than in the store for the reason every
// other door in this file gives: a caller learns WHICH field is wrong, and the
// store's own re-check answers an internal error that tells an admin nothing
// they can act on. So they need a case that runs without a database, and these
// are it — the store's half is proven against Postgres next door.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// rejectRequest is one call at the rejection door, signed in as a human — the
// only principal this operation admits.
func rejectRequest(t *testing.T, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/organizations/"+ids.NewV7().String()+"/reject",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(principal.WithActor(req.Context(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:probe", UserID: ids.NewV7(),
	}))
	return httptest.NewRecorder(), req
}

// problemOf reads the RFC 7807 body a refusal answers with.
func problemOf(t *testing.T, rec *httptest.ResponseRecorder) struct {
	Code    string `json:"code"`
	Details struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"details"`
} {
	t.Helper()
	var problem struct {
		Code    string `json:"code"`
		Details struct {
			Errors []struct {
				Field string `json:"field"`
				Code  string `json:"code"`
			} `json:"errors"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("the refusal is not a problem document: %v\n%s", err, rec.Body.String())
	}
	return problem
}

// A reason is required, and whitespace is not one.
//
// The contract says minLength 1 and a non-whitespace pattern; the generated
// type enforces neither, so this door is what makes either true. Both refusals
// name the field, because a 422 that does not is a caller guessing which of two
// values it sent was wrong.
func TestRejectingWithoutAReasonNamesTheField(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"no reason at all", `{}`},
		{"an empty reason", `{"reason":""}`},
		// The case the schema pattern now predicts and the handler has always
		// refused: trimmed, this is the empty one above.
		{"whitespace only", `{"reason":"   \t "}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec, req := rejectRequest(t, tc.body)
			Handlers{}.RejectOrganization(rec, req, crmcontracts.Id{},
				crmcontracts.RejectOrganizationParams{})

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
			}
			problem := problemOf(t, rec)
			if len(problem.Details.Errors) != 1 ||
				problem.Details.Errors[0].Field != fieldKeyReason ||
				problem.Details.Errors[0].Code != "required" {
				t.Errorf("the refusal names %+v, want one entry for %q/required",
					problem.Details.Errors, fieldKeyReason)
			}
		})
	}
}

// A reason past the contract's ceiling is refused HERE, not stored.
//
// The generated type does not enforce maxLength, so unchecked one caller stores
// a megabyte on the domain and every reader of the blocked list is served it
// back in full. The refusal says how long the ceiling is and how long this one
// was, because "too long" without either is not something a caller can act on.
func TestRejectingWithAnOverlongReasonSaysHowLongIsAllowed(t *testing.T) {
	body, err := json.Marshal(map[string]string{
		"reason": strings.Repeat("é", maxRejectReason+1),
	})
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	rec, req := rejectRequest(t, string(body))

	Handlers{}.RejectOrganization(rec, req, crmcontracts.Id{},
		crmcontracts.RejectOrganizationParams{})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	problem := problemOf(t, rec)
	if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Code != "too_long" {
		t.Fatalf("the refusal names %+v, want one entry coded too_long", problem.Details.Errors)
	}
	// RUNES, not bytes: the ceiling is a character count, and a multi-byte
	// reason measured in bytes would be refused at a third of its allowance.
	if !strings.Contains(rec.Body.String(), "501") {
		t.Errorf("the refusal reads %q and does not say how long this reason was — measured in bytes it would say 1002",
			rec.Body.String())
	}
}

// An agent is refused before anything else. The admission this verb writes is a
// HUMAN one and therefore sticky against every later machine verdict, so an
// agent minting one would launder a machine judgement into a decision no
// verdict may revisit.
func TestRejectingIsRefusedToAnAgent(t *testing.T) {
	rec, req := rejectRequest(t, `{"reason":"a tool we use"}`)
	req = req.WithContext(principal.WithActor(req.Context(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:scout",
	}))

	Handlers{}.RejectOrganization(rec, req, crmcontracts.Id{},
		crmcontracts.RejectOrganizationParams{})

	if rec.Code == http.StatusUnprocessableEntity || rec.Code < 400 {
		t.Fatalf("an agent reached the reason checks (status %d) — the human-only refusal must come first", rec.Code)
	}
}
