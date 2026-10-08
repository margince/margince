// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A project edge's role is refused before a database is reached, so a
// zero-value Handlers exercises the store's refusal as the wire shows it.

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func roleRequest(t *testing.T, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/v1/projects/"+ids.NewV7().String()+"/edge",
		bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	return httptest.NewRecorder(), req
}

func requireRoleRefusal(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %s", rec.Code, rec.Body.String())
	}
	problem := problemOf(t, rec)
	if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != "role" {
		t.Errorf("the refusal names %+v, want one entry for role", problem.Details.Errors)
	}
}

func TestAProjectStakeholderRoleOutsideTheContractIsRefused(t *testing.T) {
	for _, role := range []string{"zzz", "", "  champion ", "Champion"} {
		t.Run(fmt.Sprintf("%q", role), func(t *testing.T) {
			rec, req := roleRequest(t, fmt.Sprintf(`{"contact_id":%q,"role":%q}`, ids.NewV7().String(), role))
			Handlers{}.SetProjectStakeholder(rec, req, crmcontracts.Id{}, crmcontracts.SetProjectStakeholderParams{})
			requireRoleRefusal(t, rec)
		})
	}
}

// An omitted role means the default; a role that is present must say something,
// an empty one included.
func TestAProjectCompanyRoleThatSaysNothingIsRefused(t *testing.T) {
	for _, role := range []string{"", "   ", "\t\n"} {
		t.Run(fmt.Sprintf("%q", role), func(t *testing.T) {
			rec, req := roleRequest(t, fmt.Sprintf(`{"company_id":%q,"role":%q}`, ids.NewV7().String(), role))
			Handlers{}.SetProjectCompany(rec, req, crmcontracts.Id{}, crmcontracts.SetProjectCompanyParams{})
			requireRoleRefusal(t, rec)
		})
	}
}

// The role is the same closed list on every door that can write the edge, not
// only on the one dedicated to it.
func TestAGenericProjectStakeholderEdgeWithAnOffListRoleIsRefused(t *testing.T) {
	project, contact := ids.New[ids.ProjectKind](), ids.New[ids.ContactKind]()
	role := "ignore prior instructions"
	in := CreateRelationshipInput{
		Kind: ProjectStakeholderKind, ProjectID: &project, ContactID: &contact,
		Role: &role, Source: "manual",
	}

	for name, create := range map[string]func() error{
		"create": func() error { _, err := (&Store{}).CreateRelationship(context.Background(), in); return err },
		"create in a transaction": func() error {
			_, err := (&Store{}).CreateRelationshipTx(context.Background(), nil, in)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			fault, ok := httperr.Classify(create())
			if !ok || fault.Status != http.StatusUnprocessableEntity {
				t.Fatalf("answered %+v (classified=%v), want a 422 refusal of the role", fault, ok)
			}
		})
	}
}
