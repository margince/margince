// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type fieldFaults struct {
	Details struct {
		Errors []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"details"`
}

func (f fieldFaults) only() (string, string) {
	if len(f.Details.Errors) != 1 {
		return "", ""
	}
	return f.Details.Errors[0].Field, f.Details.Errors[0].Code
}

// Each door names the refusal on its own field. The typed create uses emails,
// and quick capture uses email.
func TestEachContactDoorRefusesASendingSystemsAddressOnItsOwnField(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	for _, c := range []struct {
		door, path  string
		body        AnyMap
		field, code string
	}{
		{
			door: "typed create", path: "/v1/contacts", field: "emails", code: "email_names_no_one",
			body: AnyMap{"full_name": "Signing", "source": "manual", "emails": []AnyMap{{"email": "signing-noreply@doors.example", "is_primary": true}}},
		},
		{
			door: "quick capture", path: "/v1/contacts/quick-capture", field: "email", code: "email_names_no_one",
			body: AnyMap{"full_name": "Receipts", "email": "receipts@doors.example"},
		},
	} {
		var faults fieldFaults
		if status := e.Call(t, http.MethodPost, c.path, c.body, nil, &faults); status != http.StatusUnprocessableEntity {
			t.Errorf("%s → %d, want 422", c.door, status)
			continue
		}
		if field, code := faults.only(); field != c.field || code != c.code {
			t.Errorf("%s fault = %q %q, want %q %q", c.door, field, code, c.field, c.code)
		}
	}
	if got := importedContactCount(t, e); got != 0 {
		t.Errorf("refused creates left %d contacts", got)
	}
}
