// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A REST archive of a record type the archive_record TOOL cannot even express
// still stages against the row it names.
//
// `tag` is outside that tool's declared enum, and outside the record seam's
// vocabulary too — nothing the tool door can reach describes it. The REST door
// archives it all the same, so it is the operation, not the tool's schema, that
// decides what a governed call is about. The proof has to be the approval ROW:
// ErrRequiresApproval comes back from a refusal with nowhere to land as
// readily as from a staged one, and a target_entity_id that is merely a
// well-formed uuid is not the record anybody will decide about.

import (
	"net/http"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
)

// A REST route the tool schema does not cover archives on the agent's own
// passport, and the row is gone.
//
// `archive_record` does not serve `tag` — the tool's own vocabulary stops
// short of it — so this route is reachable only over REST, and it used to stage
// there. It no longer does: a passport carries the granting human's seat and
// row scope, and archiving a tag is ordinary work its holder does unaided.
//
// What the staging used to prove — that a row outside the tool's vocabulary is
// still named correctly when a floor puts it back behind an approval — is held
// by TestARestCreateStagesItsRecordTypeWithNoTargetID on a route that still
// carries its floor.
func TestARestArchiveOutsideTheToolSchemaPerformsOnTheAgentsPassport(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	bearer := agentBearer(t, e, "outside-the-tool-schema agent")

	tagID := createdID(t, e, "/v1/tags", apptest.AnyMap{"name": "Champion"})

	if status := e.Call(t, "DELETE", "/v1/tags/"+tagID, nil, bearer, nil); status == http.StatusForbidden {
		t.Fatalf("agent tag archive → 403 — a passport archives what its holder could archive unaided")
	}

	var archived bool
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT archived_at IS NOT NULL FROM tag WHERE id = $1`, tagID).Scan(&archived); err != nil {
		t.Fatalf("reading the tag back: %v", err)
	}
	if !archived {
		t.Error("the tag is still live after the agent archived it")
	}
}
