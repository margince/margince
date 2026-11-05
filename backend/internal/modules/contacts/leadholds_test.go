// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALeadHeldRefusalReachesTheWireAsTheDuplicateProblem(t *testing.T) {
	t.Parallel()
	lead := ids.New[ids.LeadKind]()
	refusal := fmt.Errorf("create: %w", &LeadHoldsIdentityError{Field: fieldEmails, LeadIDs: []ids.LeadID{lead}, Convertible: true})
	rec := httptest.NewRecorder()
	writeStoreErr(rec, httptest.NewRequest(http.MethodPost, "/v1/contacts", nil), refusal)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409", rec.Code)
	}
	var body struct {
		Code    string `json:"code"`
		Details struct {
			ExistingID  string   `json:"existing_id"`
			LeadIDs     []string `json:"lead_ids"`
			Field       string   `json:"field"`
			Convertible bool     `json:"convertible"`
		} `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != CodeLeadHoldsEmail || body.Details.ExistingID != lead.String() ||
		len(body.Details.LeadIDs) != 1 || body.Details.LeadIDs[0] != lead.String() ||
		body.Details.Field != fieldEmails || !body.Details.Convertible {
		t.Fatalf("problem = %s", rec.Body.String())
	}
}

func TestALeadHeldRefusalNamingNoLeadSaysNothingMore(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeStoreErr(rec, httptest.NewRequest(http.MethodPost, "/v1/contacts", nil), &LeadHoldsIdentityError{LeadIDs: []ids.LeadID{}})
	var body struct {
		Details map[string]any `json:"details"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Details) != 0 {
		t.Fatalf("a refusal naming no lead must name no lead, key or convert: %s", rec.Body.String())
	}
}

func TestOnlyAReadableProfileURLIsALinkedInKey(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"http://WWW.LinkedIn.com/in/sam/?trk=x": "https://www.linkedin.com/in/sam",
		"linkedin.com/in/sam":                   "https://www.linkedin.com/in/sam",
		"sam":                                   "",
		"@sam":                                  "",
	} {
		got, ok := linkedInIdentity(map[string]any{SocialLinkedIn: raw})
		if (want == "") == ok || (ok && got != want) {
			t.Errorf("linkedInIdentity(%q) = %q, %v; want %q", raw, got, ok, want)
		}
	}
}

// A door outside this module, the review accept answered through approvals,
// still says which refusal it was.
func TestTheIdentityRefusalsKeepTheirCodeOnTheSharedMapping(t *testing.T) {
	t.Parallel()
	for code, refusal := range map[string]error{
		CodeLeadHoldsEmail:    fmt.Errorf("accept: %w", &LeadHoldsIdentityError{Field: fieldEmails}),
		CodeContactHoldsEmail: fmt.Errorf("create: %w", &ContactHoldsIdentityError{}),
	} {
		rec := httptest.NewRecorder()
		httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/v1/approvals/x/decide", nil), refusal)
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict || body.Code != code {
			t.Errorf("%s on the shared mapping = %d %s, want 409 %s", code, rec.Code, rec.Body.String(), code)
		}
	}
}

func TestAContactHeldRefusalNamesOnlyAReadableContact(t *testing.T) {
	t.Parallel()
	contact := ids.New[ids.ContactKind]()
	for name, tc := range map[string]struct {
		refusal  *ContactHoldsIdentityError
		existing string
		field    string
	}{
		"readable": {&ContactHoldsIdentityError{Field: leadEmailColumn, ContactID: contact, OwnerNotified: true}, contact.String(), leadEmailColumn},
		"hidden":   {&ContactHoldsIdentityError{OwnerNotified: true, holder: contact}, "", ""},
	} {
		rec := httptest.NewRecorder()
		writeStoreErr(rec, httptest.NewRequest(http.MethodPost, "/v1/leads", nil), fmt.Errorf("create: %w", tc.refusal))
		var body struct {
			Code    string `json:"code"`
			Details struct {
				ExistingID    string `json:"existing_id"`
				Field         string `json:"field"`
				OwnerNotified bool   `json:"owner_notified"`
			} `json:"details"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusConflict || body.Code != CodeContactHoldsEmail || body.Details.ExistingID != tc.existing ||
			body.Details.Field != tc.field || !body.Details.OwnerNotified {
			t.Errorf("%s refusal = %d %s", name, rec.Code, rec.Body.String())
		}
	}
}
