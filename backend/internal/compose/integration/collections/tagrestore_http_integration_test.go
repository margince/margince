// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestATagRemovalAnswersTheHandleThatPutsItBack(t *testing.T) {
	e := tagEnv(t)
	tag := createTag(t, e, "Undo Over The Wire")
	contact := createContactWithTag(t, e, "Tagged Then Not", tag)
	link := integration.AnyMap{"entity_type": "contact", "entity_id": contact}

	var undo integration.AnyMap
	if status := e.Call(t, "DELETE", "/v1/tags/"+tag+"/apply", link, nil, &undo); status != http.StatusOK || undo["audit_id"] == nil {
		t.Fatalf("removing a tagging that was there → %d %v, want 200 with its audit_id", status, undo)
	}
	if status := e.Call(t, "DELETE", "/v1/tags/"+tag+"/apply", link, nil, nil); status != http.StatusNoContent {
		t.Fatalf("removing a tagging that is not there → %d, want 204", status)
	}

	var back integration.AnyMap
	if status := e.Call(t, "POST", "/v1/tags/"+tag+"/apply/restore", undo, nil, &back); status != http.StatusOK {
		t.Fatalf("restoring the removal → %d %v, want 200", status, back)
	}
	if back["entity_id"] != contact || back["tag_id"] != tag {
		t.Fatalf("the restore answered %v, want the tagging of %s by %s", back, contact, tag)
	}
	if status := e.Call(t, "POST", "/v1/tags/"+tag+"/apply/restore", undo, nil, nil); status != http.StatusConflict {
		t.Fatalf("restoring the same removal twice → %d, want 409", status)
	}
}
