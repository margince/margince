// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package identifierspellingit_test holds the two input rules end to end. An
// address typed two ways is one address. A name that would show in another
// order than stored is refused.
package identifierspellingit_test

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type body = map[string]any

func createContact(t *testing.T, e *apptest.AppEnv, fields body) (int, string) {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	fields["source"] = "manual"
	return e.Call(t, "POST", "/v1/contacts", fields, nil, &created), created.ID
}

// storedEmails reads back a contact's addresses as the API serves them.
func storedEmails(t *testing.T, e *apptest.AppEnv, id string) []string {
	t.Helper()
	var contact struct {
		Emails []struct {
			Email string `json:"email"`
		} `json:"emails"`
	}
	if status := e.Call(t, "GET", "/v1/contacts/"+id, nil, nil, &contact); status != http.StatusOK {
		t.Fatalf("reading contact %s → %d", id, status)
	}
	out := make([]string, 0, len(contact.Emails))
	for _, row := range contact.Emails {
		out = append(out, row.Email)
	}
	return out
}

func TestAnAddressTypedWithAHiddenCharacterAndAnAccentIsTheSameAddress(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Spelling", "admin@spelling.test", "Admin")
	const composed = "jos\u00e9@buyer.test"
	typed := "\u200bjose\u0301@buyer.test\ufeff"
	status, id := createContact(t, e, body{"full_name": "Jose Ruiz", "emails": []body{{"email": typed, "email_type": "work", "is_primary": true}}})
	if status != http.StatusCreated {
		t.Fatalf("creating the contact → %d", status)
	}
	if got := storedEmails(t, e, id); len(got) != 1 || got[0] != composed {
		t.Errorf("the address was stored as %q, want the one spelling %q", got, composed)
	}
}

// A name refused here shows on screen in another order than the one stored.
func TestANameThatReversesItsOrderIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Names", "admin@names.test", "Admin")
	const reversed = "Invoice \u202egnp.exe"
	for _, write := range []struct {
		path, field string
		rest        body
	}{
		{"/v1/contacts", "full_name", body{"source": "manual", "linkedin_url": apptest.ContactProfile()}},
		{"/v1/teams", "name", body{}},
		{"/v1/pipelines", "name", body{}},
	} {
		var refusal struct {
			Details struct {
				Errors []struct{ Field, Code string } `json:"errors"`
			} `json:"details"`
		}
		write.rest[write.field] = reversed
		status := e.Call(t, "POST", write.path, write.rest, nil, &refusal)
		refused := refusal.Details.Errors
		if status != http.StatusUnprocessableEntity || len(refused) != 1 ||
			refused[0].Field != write.field || refused[0].Code != "invalid_characters" {
			t.Errorf("POST %s with a direction override in %s → %d %+v, want 422 invalid_characters on %s",
				write.path, write.field, status, refused, write.field)
		}
	}
	if status, _ := createContact(t, e, body{"linkedin_url": apptest.ContactProfile(), "full_name": "Đội José"}); status != http.StatusCreated {
		t.Errorf("a name with accents → %d, want 201", status)
	}
}
