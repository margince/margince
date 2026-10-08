// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The form that says who the controller is opens empty, saves, and refuses a
// value the entry's own validator rejects.
func TestTheControllerParticularsFormOpensSavesAndRefuses(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Controller Particulars", "admin@particulars.test", "Admin")
	const path = "/v1/privacy/controller-particulars"

	var empty AnyMap
	if status := e.Call(t, "GET", path, nil, nil, &empty); status != http.StatusOK {
		t.Fatalf("reading the form answered %d, want 200 with empty fields", status)
	}

	var saved struct {
		LegalName string `json:"legal_name"`
	}
	if status := e.Call(t, "PUT", path, AnyMap{"legal_name": "Acme GmbH"}, nil, &saved); status != http.StatusOK {
		t.Fatalf("saving the form answered %d, want 200", status)
	}
	if saved.LegalName != "Acme GmbH" {
		t.Errorf("the saved legal name reads back as %q, want the stored value", saved.LegalName)
	}

	var problem problemBody
	if status := e.Call(t, "PUT", path, AnyMap{"legal_name": "   "}, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Errorf("a blank legal name answered %d (%s), want 422", status, problem.Code)
	}
}
