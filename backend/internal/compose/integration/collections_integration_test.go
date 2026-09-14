// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Lists & tags over HTTP: the organizational surfaces respect the same
// laws as everything else — member/tag references are reads of
// row-scoped records (H1 probe), duplicates answer 409, dynamic
// segments refuse manual members, archived tags read as absent.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func setupCollections(t *testing.T) (*apptest.AppEnv, string) {
	t.Helper()
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Collections E2E", "company@fable.test", "Admin")
	var contact struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": "List Target"}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	return e, contact.ID
}

func TestTagsLifecycleAndApplication(t *testing.T) {
	e, contactID := setupCollections(t)

	var tag struct {
		ID string `json:"id"`
	}
	// A palette tone, not a hex: colour became a closed set of four when the tag
	// vocabulary was made governed, and free text is refused.
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "Champion", "color": "amber"}, nil, &tag); status != http.StatusCreated {
		t.Fatalf("create tag → %d", status)
	}
	// The name is unique case-insensitively.
	if status := e.Call(t, "POST", "/v1/tags", AnyMap{"name": "champion"}, nil, nil); status != http.StatusConflict {
		t.Fatalf("duplicate tag name → %d, want 409", status)
	}

	if status := e.Call(t, "POST", "/v1/tags/"+tag.ID+"/apply", AnyMap{
		"entity_type": "contact", "entity_id": contactID,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("apply tag → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/tags/"+tag.ID+"/apply", AnyMap{
		"entity_type": "contact", "entity_id": contactID,
	}, nil, nil); status != http.StatusConflict {
		t.Fatalf("re-apply → %d, want 409", status)
	}

	if status := e.Call(t, "DELETE", "/v1/tags/"+tag.ID, nil, nil, nil); status != http.StatusOK {
		t.Fatalf("archive tag → %d", status)
	}
	// An archived tag reads as absent for new applications.
	if status := e.Call(t, "POST", "/v1/tags/"+tag.ID+"/apply", AnyMap{
		"entity_type": "contact", "entity_id": contactID,
	}, nil, nil); status != http.StatusNotFound {
		t.Fatalf("apply on archived tag → %d, want 404", status)
	}
}
