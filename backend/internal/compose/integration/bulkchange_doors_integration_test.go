// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A bulk change through both of its doors: POST /v1/bulk/preview and
// /v1/bulk/execute as a signed-in human, and the bulk_update_records tool as an
// agent over /mcp. Every assertion runs on both, because the claim is that they
// are one engine — a door that let eleven records through without a token, or
// spent a token twice, would be the other door's gap.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/redistest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type bulkItemDTO struct {
	ID      string `json:"id"`
	Version int64  `json:"version"`
}

type bulkSkipDTO struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type bulkPreviewDTO struct {
	Count                int           `json:"count"`
	Excluded             []bulkSkipDTO `json:"excluded"`
	RequiresConfirmation bool          `json:"requires_confirmation"`
	ConfirmToken         string        `json:"confirm_token"`
}

type bulkResultDTO struct {
	BatchID string        `json:"batch_id"`
	Changed int           `json:"changed"`
	Skipped []bulkSkipDTO `json:"skipped"`
}

// bulkDoor is one way in. A refusal comes back as the text the caller is
// shown, empty when the call ran.
type bulkDoor struct {
	name    string
	preview func(t *testing.T, change AnyMap) (bulkPreviewDTO, string)
	execute func(t *testing.T, change AnyMap, retryKey string) (bulkResultDTO, string)
}

func httpBulkDoor(e *apptest.AppEnv) bulkDoor {
	call := func(t *testing.T, path string, change AnyMap, headers map[string]string, out any) string {
		t.Helper()
		var raw json.RawMessage
		status := e.Call(t, "POST", path, change, headers, &raw)
		if status != http.StatusOK {
			return string(raw)
		}
		if err := json.Unmarshal(raw, out); err != nil {
			t.Fatalf("%s answered a body that does not decode: %v", path, err)
		}
		return ""
	}
	return bulkDoor{
		name: "http",
		preview: func(t *testing.T, change AnyMap) (out bulkPreviewDTO, refusal string) {
			return out, call(t, "/v1/bulk/preview", change, nil, &out)
		},
		execute: func(t *testing.T, change AnyMap, retryKey string) (out bulkResultDTO, refusal string) {
			var headers map[string]string
			if retryKey != "" {
				headers = map[string]string{"Idempotency-Key": retryKey}
			}
			return out, call(t, "/v1/bulk/execute", change, headers, &out)
		},
	}
}

func mcpBulkDoor(client *apptest.MCPClient) bulkDoor {
	call := func(t *testing.T, mode string, change AnyMap, retryKey string, out any) string {
		t.Helper()
		args := AnyMap{"mode": mode}
		for k, v := range change {
			args[k] = v
		}
		if retryKey != "" {
			args["idempotency_key"] = retryKey
		}
		got := client.Call(t, "bulk_update_records", args)
		if got.IsError {
			return got.Text
		}
		got.JSON(t, out)
		return ""
	}
	return bulkDoor{
		name: "mcp",
		preview: func(t *testing.T, change AnyMap) (out bulkPreviewDTO, refusal string) {
			return out, call(t, "preview", change, "", &out)
		},
		execute: func(t *testing.T, change AnyMap, retryKey string) (out bulkResultDTO, refusal string) {
			return out, call(t, "execute", change, retryKey, &out)
		},
	}
}

// bulkDoorsApp boots the stack with the tool surface mounted and a live volume
// meter, and answers both doors over it.
func bulkDoorsApp(t *testing.T, slug string, limits agentvolume.Limits) (*apptest.AppEnv, *agentvolume.Meter, []bulkDoor) {
	t.Helper()
	meter := agentvolume.New(redistest.Client(t), limits, agentvolume.DefaultWindow)
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"),
			compose.WithAgentVolume(meter),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Bulk Doors", slug+"@fable.test", "Admin")
	client := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "bulk agent", "read", "write"))
	return e, meter, []bulkDoor{httpBulkDoor(e), mcpBulkDoor(client)}
}

// seedColleague adds an active human seat the admin may hand records to. No
// endpoint makes a seat active without a sign-in, so the row is written here.
func seedColleague(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	id := ids.NewV7().String()
	if _, err := e.Owner.Exec(t.Context(),
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		id, "colleague-"+id+"@fable.test"); err != nil {
		t.Fatalf("seeding a colleague: %v", err)
	}
	return id
}

// seedBulkContacts creates n contacts through the contact endpoint and answers
// each as a list row would carry it.
func seedBulkContacts(t *testing.T, e *apptest.AppEnv, n int) []bulkItemDTO {
	t.Helper()
	items := make([]bulkItemDTO, 0, n)
	for i := range n {
		var created bulkItemDTO
		if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
			"full_name": "Bulk Door Contact " + ids.NewV7().String(),
		}, nil, &created); status != http.StatusCreated {
			t.Fatalf("seeding contact %d → %d", i, status)
		}
		items = append(items, created)
	}
	return items
}

func reassignContacts(owner string, items []bulkItemDTO) AnyMap {
	return AnyMap{"record_type": "contact", "verb": "reassign_owner", "owner_id": owner, "items": items}
}

func TestBothDoorsChangeASmallSelectionDirectlyUnderOneBatch(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-small", agentvolume.Limits{})
	colleague := seedColleague(t, e)
	for _, door := range doors {
		items := seedBulkContacts(t, e, 5)
		out, refusal := door.execute(t, reassignContacts(colleague, items), "")
		if refusal != "" || out.Changed != 5 {
			t.Fatalf("%s: five contacts → %+v, refused %q; want all five changed without a preview", door.name, out, refusal)
		}
		if n := countOwned(t, e, `SELECT count(*) FROM audit_log WHERE batch_id = $1 AND action = 'update'`, out.BatchID); n != 5 {
			t.Errorf("%s: %d audit rows carry the batch id, want five", door.name, n)
		}
		assertTheAuditLogFindsTheBatch(t, e, out.BatchID, 5)
	}
}

// The compliance read narrows to one batch, which is how an administrator finds
// every record one bulk change touched.
func assertTheAuditLogFindsTheBatch(t *testing.T, e *apptest.AppEnv, batchID string, want int) {
	t.Helper()
	var page struct {
		Data []struct {
			BatchID string `json:"batch_id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/audit-log?batch_id="+batchID+"&limit=50", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET /audit-log?batch_id → %d", status)
	}
	if len(page.Data) != want {
		t.Errorf("the audit log names %d rows for the batch, want %d", len(page.Data), want)
	}
	for _, row := range page.Data {
		if row.BatchID != batchID {
			t.Errorf("a row of another batch (%q) came back for %q", row.BatchID, batchID)
		}
	}
}

func TestBothDoorsNeedTheirPreviewsTokenAboveTenAndSpendItOnce(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-token", agentvolume.Limits{})
	colleague, other := seedColleague(t, e), seedColleague(t, e)
	for _, door := range doors {
		items := seedBulkContacts(t, e, 25)
		change := reassignContacts(colleague, items)
		if _, refusal := door.execute(t, change, ""); !strings.Contains(refusal, "confirm_token") {
			t.Fatalf("%s: twenty-five without a token → %q, want a refusal naming confirm_token", door.name, refusal)
		}
		preview, refusal := door.preview(t, change)
		if refusal != "" || !preview.RequiresConfirmation || preview.ConfirmToken == "" || preview.Count != 25 {
			t.Fatalf("%s: preview → %+v, refused %q; want 25 affected and a token", door.name, preview, refusal)
		}
		elsewhere := reassignContacts(other, items)
		elsewhere["confirm_token"] = preview.ConfirmToken
		if _, refusal := door.execute(t, elsewhere, ""); !strings.Contains(refusal, "confirm_token") {
			t.Errorf("%s: the token presented for another owner → %q, want it refused", door.name, refusal)
		}
		change["confirm_token"] = preview.ConfirmToken
		if out, refusal := door.execute(t, change, ""); refusal != "" || out.Changed != 25 {
			t.Fatalf("%s: executing with the token → %+v, refused %q", door.name, out, refusal)
		}
		if _, refusal := door.execute(t, change, ""); !strings.Contains(refusal, "confirm_token") {
			t.Errorf("%s: the spent token → %q, want it refused", door.name, refusal)
		}
	}
}

func TestBothDoorsSkipARecordEditedAfterThePreview(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-stale", agentvolume.Limits{})
	colleague := seedColleague(t, e)
	for _, door := range doors {
		items := seedBulkContacts(t, e, 2)
		change := reassignContacts(colleague, items)
		if _, refusal := door.preview(t, change); refusal != "" {
			t.Fatalf("%s: preview refused %q", door.name, refusal)
		}
		if status := e.Call(t, "PATCH", "/v1/contacts/"+items[1].ID, AnyMap{"title": "Buyer"}, nil, nil); status != http.StatusOK {
			t.Fatalf("editing the contact after the preview → %d", status)
		}
		out, refusal := door.execute(t, change, "")
		if refusal != "" || out.Changed != 1 || len(out.Skipped) != 1 || out.Skipped[0].Reason != "changed_since_preview" {
			t.Errorf("%s: → %+v, refused %q; want the edited contact skipped as changed_since_preview", door.name, out, refusal)
		}
	}
}

// A retried execution is never a second change. The REST door hands back the
// first answer; the tool door refuses to replay an answer that names no record
// it can re-check, which the tool surface does for every such answer.
func TestARetriedExecutionIsNotASecondChange(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-retry", agentvolume.Limits{})
	colleague := seedColleague(t, e)
	for _, door := range doors {
		items := seedBulkContacts(t, e, 3)
		key := "bulk-retry-" + door.name
		first, refusal := door.execute(t, reassignContacts(colleague, items), key)
		if refusal != "" {
			t.Fatalf("%s: first execution refused %q", door.name, refusal)
		}
		again, _ := door.execute(t, reassignContacts(colleague, items), key)
		if door.name == "http" && again.BatchID != first.BatchID {
			t.Errorf("http: the retry answered batch %q, want the first answer's %q", again.BatchID, first.BatchID)
		}
		if n := countOwned(t, e, `SELECT count(*) FROM bulk_operation WHERE id <> $1 AND changed_count > 0
			AND created_at >= (SELECT created_at FROM bulk_operation WHERE id = $1)`, first.BatchID); n != 0 {
			t.Errorf("%s: the retry ran the change again (%d further batches)", door.name, n)
		}
	}
}

func TestBothDoorsReassignAndArchiveDeals(t *testing.T) {
	e, _, doors := bulkDoorsApp(t, "bulk-deals", agentvolume.Limits{})
	colleague := seedColleague(t, e)
	stages := apptest.DiscoverSeededPipeline(t, e)
	for _, door := range doors {
		deals := make([]bulkItemDTO, 0, 2)
		for range 2 {
			var deal bulkItemDTO
			if status := e.Call(t, "POST", "/v1/deals", AnyMap{
				"name": "Bulk deal " + ids.NewV7().String(), "pipeline_id": stages.PipelineID,
				"stage_id": stages.Open, "source": "manual",
			}, nil, &deal); status != http.StatusCreated {
				t.Fatalf("seeding a deal → %d", status)
			}
			deals = append(deals, deal)
		}
		moved, refusal := door.execute(t, AnyMap{"record_type": "deal", "verb": "reassign_owner", "owner_id": colleague, "items": deals}, "")
		if refusal != "" || moved.Changed != 2 {
			t.Fatalf("%s: reassigning two deals → %+v, refused %q", door.name, moved, refusal)
		}
		for i := range deals {
			deals[i].Version++
		}
		archived, refusal := door.execute(t, AnyMap{"record_type": "deal", "verb": "archive", "items": deals}, "")
		if refusal != "" || archived.Changed != 2 {
			t.Errorf("%s: archiving two deals → %+v, refused %q", door.name, archived, refusal)
		}
	}
}

func countOwned(t *testing.T, e *apptest.AppEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(), query, args...).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}
