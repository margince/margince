// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Undoing a bulk change through both doors: POST /v1/bulk/{id}/undo/preview
// and /v1/bulk/{id}/undo as a signed-in human, and bulk_update_records'
// undo_preview and undo modes as an agent over /mcp. The same assertions run on
// both, because they are one engine.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
)

type bulkUndoResultDTO struct {
	bulkResultDTO
	UndoOf string `json:"undo_of"`
}

// undoDoor is one way into the undo. A refusal comes back as the text the
// caller is shown, empty when the call ran.
type undoDoor struct {
	bulkDoor
	preview func(t *testing.T, batchID string) (bulkPreviewDTO, string)
	undo    func(t *testing.T, batchID, token string) (bulkUndoResultDTO, string)
}

func undoDoors(e *apptest.AppEnv, doors []bulkDoor, client *apptest.MCPClient) []undoDoor {
	httpDoor := undoDoor{
		bulkDoor: doors[0],
		preview: func(t *testing.T, batchID string) (out bulkPreviewDTO, refusal string) {
			raw, refusal := httpPost(t, e, "/v1/bulk/"+batchID+"/undo/preview", nil)
			if refusal == "" && json.Unmarshal(raw, &out) != nil {
				t.Fatalf("the undo preview answered a body that does not decode: %s", raw)
			}
			return out, refusal
		},
		undo: func(t *testing.T, batchID, token string) (out bulkUndoResultDTO, refusal string) {
			body := AnyMap{}
			if token != "" {
				body["confirm_token"] = token
			}
			raw, refusal := httpPost(t, e, "/v1/bulk/"+batchID+"/undo", body)
			if refusal == "" && json.Unmarshal(raw, &out) != nil {
				t.Fatalf("the undo answered a body that does not decode: %s", raw)
			}
			return out, refusal
		},
	}
	mcpCall := func(t *testing.T, args AnyMap, out any) string {
		t.Helper()
		got := client.Call(t, "bulk_update_records", args)
		if got.IsError {
			return got.Text
		}
		got.JSON(t, out)
		return ""
	}
	mcpDoor := undoDoor{
		bulkDoor: doors[1],
		preview: func(t *testing.T, batchID string) (out bulkPreviewDTO, refusal string) {
			return out, mcpCall(t, AnyMap{"mode": "undo_preview", "batch_id": batchID}, &out)
		},
		undo: func(t *testing.T, batchID, token string) (out bulkUndoResultDTO, refusal string) {
			args := AnyMap{"mode": "undo", "batch_id": batchID}
			if token != "" {
				args["confirm_token"] = token
			}
			return out, mcpCall(t, args, &out)
		},
	}
	return []undoDoor{httpDoor, mcpDoor}
}

// httpPost answers the body of a 200, or the refusal the caller is shown.
func httpPost(t *testing.T, e *apptest.AppEnv, path string, body AnyMap) (json.RawMessage, string) {
	t.Helper()
	var raw json.RawMessage
	if status := e.Call(t, "POST", path, body, nil, &raw); status != http.StatusOK {
		return nil, string(raw)
	}
	return raw, ""
}

func bulkUndoApp(t *testing.T, slug string) (*apptest.AppEnv, []undoDoor) {
	t.Helper()
	e, _, doors := bulkDoorsApp(t, slug, agentvolume.Limits{})
	client := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "bulk undo agent", "read", "write"))
	return e, undoDoors(e, []bulkDoor{doors[0], mcpBulkDoor(client)}, client)
}

func contactsLive(t *testing.T, e *apptest.AppEnv, items []bulkItemDTO) int {
	t.Helper()
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.ID
	}
	return countOwned(t, e, `SELECT count(*) FROM contact WHERE id::text = ANY($1) AND archived_at IS NULL`, ids)
}

func TestBothDoorsUndoABulkArchiveOnce(t *testing.T) {
	e, doors := bulkUndoApp(t, "bulk-undo")
	for _, door := range doors {
		items := seedBulkContacts(t, e, 3)
		archived, refusal := door.execute(t, AnyMap{"record_type": "contact", "verb": "archive", "items": items}, "")
		if refusal != "" || archived.Changed != 3 {
			t.Fatalf("%s: archiving three → %+v, refused %q", door.name, archived, refusal)
		}
		undone, refusal := door.undo(t, archived.BatchID, "")
		if refusal != "" || undone.Changed != 3 || undone.UndoOf != archived.BatchID || undone.BatchID == archived.BatchID {
			t.Fatalf("%s: undo → %+v, refused %q; want all three back under a batch of its own", door.name, undone, refusal)
		}
		if n := contactsLive(t, e, items); n != 3 {
			t.Errorf("%s: %d of three contacts are live after the undo", door.name, n)
		}
		if n := countOwned(t, e, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND action = 'restore'`, undone.BatchID); n != 3 {
			t.Errorf("%s: %d restore audit rows carry the undo's batch id, want three", door.name, n)
		}
		if _, refusal := door.undo(t, archived.BatchID, ""); !strings.Contains(refusal, "already undone") {
			t.Errorf("%s: a second undo → %q, want it refused as already undone", door.name, refusal)
		}
	}
}

func TestBothDoorsNeedTheUndoPreviewsTokenAboveTen(t *testing.T) {
	e, doors := bulkUndoApp(t, "bulk-undo-token")
	colleague := seedColleague(t, e)
	for _, door := range doors {
		change := reassignContacts(colleague, seedBulkContacts(t, e, 11))
		preview, refusal := door.bulkDoor.preview(t, change)
		if refusal != "" {
			t.Fatalf("%s: preview refused %q", door.name, refusal)
		}
		change["confirm_token"] = preview.ConfirmToken
		moved, refusal := door.execute(t, change, "")
		if refusal != "" || moved.Changed != 11 {
			t.Fatalf("%s: reassigning eleven → %+v, refused %q", door.name, moved, refusal)
		}
		if _, refusal := door.undo(t, moved.BatchID, ""); !strings.Contains(refusal, "confirm_token") {
			t.Errorf("%s: undoing eleven without a token → %q, want a refusal naming confirm_token", door.name, refusal)
		}
		undoPreview, refusal := door.preview(t, moved.BatchID)
		if refusal != "" || undoPreview.Count != 11 || undoPreview.ConfirmToken == "" {
			t.Fatalf("%s: undo preview → %+v, refused %q", door.name, undoPreview, refusal)
		}
		if undone, refusal := door.undo(t, moved.BatchID, undoPreview.ConfirmToken); refusal != "" || undone.Changed != 11 {
			t.Errorf("%s: undoing with the token → %+v, refused %q", door.name, undone, refusal)
		}
	}
}

func TestTheStatusReadAnswersWhatABatchDidAndWhoUndidIt(t *testing.T) {
	e, doors := bulkUndoApp(t, "bulk-status")
	web := doors[0]
	archived, refusal := web.execute(t, AnyMap{"record_type": "contact", "verb": "archive", "items": seedBulkContacts(t, e, 2)}, "")
	if refusal != "" {
		t.Fatalf("archiving → refused %q", refusal)
	}
	undone, refusal := web.undo(t, archived.BatchID, "")
	if refusal != "" {
		t.Fatalf("undo → refused %q", refusal)
	}
	var status struct {
		Verb     string `json:"verb"`
		Changed  int    `json:"changed"`
		UndoneBy string `json:"undone_by"`
	}
	if got := e.Call(t, "GET", "/v1/bulk/"+archived.BatchID, nil, nil, &status); got != http.StatusOK {
		t.Fatalf("GET /v1/bulk/{id} → %d", got)
	}
	if status.Verb != "archive" || status.Changed != 2 || status.UndoneBy != undone.BatchID {
		t.Errorf("status → %+v; want the archive of two, undone by %s", status, undone.BatchID)
	}
}
