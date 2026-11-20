// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// listsOnApp is a workspace whose lists are switched on, because the archive
// this file restores took the deal off two of them.
func listsOnApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOptions(t, compose.WithListsEnabled(true))
	apptest.BootstrapWorkspaceSession(t, e, "Deal restore", "restore-"+ids.NewV7().String()[:8]+"@fable.test", "Admin")
	return e
}

// archivedDealFixture is a deal on two deal lists, with a stakeholder, that
// was then archived. The second list was archived after the deal, so its
// membership is the one link a restore must leave behind.
type archivedDealFixture struct {
	deal, kept, retired, stakeholder, archive string
}

func archiveADealOnTwoLists(t *testing.T, e *apptest.AppEnv, name string) archivedDealFixture {
	t.Helper()
	stages := apptest.DiscoverSeededPipeline(t, e)
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": name + " customer", "source": "manual"})
	contact := createdID(t, e, "/v1/contacts", AnyMap{"linkedin_url": apptest.ContactProfile(), "source": "manual", "full_name": name + " buyer"})
	f := archivedDealFixture{deal: createdID(t, e, "/v1/deals", AnyMap{
		"name": name, "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "company_id": company, "source": "manual",
	})}
	f.stakeholder = createdID(t, e, "/v1/relationships", AnyMap{
		"kind": "deal_stakeholder", "deal_id": f.deal, "contact_id": contact, "source": "manual",
	})
	f.kept = createdID(t, e, "/v1/lists", AnyMap{"name": name + " shortlist", "entity_type": "deal"})
	f.retired = createdID(t, e, "/v1/lists", AnyMap{"name": name + " old drive", "entity_type": "deal"})
	for _, list := range []string{f.kept, f.retired} {
		if status := e.Call(t, "POST", "/v1/lists/"+list+"/members",
			AnyMap{"entity_type": "deal", "entity_id": f.deal, "note": "from the review"}, nil, nil); status != http.StatusCreated {
			t.Fatalf("add the deal to list %s → %d", list, status)
		}
	}
	if status := e.Call(t, "DELETE", "/v1/deals/"+f.deal, nil, nil, nil); status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("archive the deal → %d", status)
	}
	if status := e.Call(t, "DELETE", "/v1/lists/"+f.retired, nil, nil, nil); status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("archive the second list → %d", status)
	}
	f.archive = theArchiveEntry(t, e, f.deal)
	return f
}

func theArchiveEntry(t *testing.T, e *apptest.AppEnv, deal string) string {
	t.Helper()
	for _, entry := range readHistory(t, e, "deal", deal).Data {
		if entry.Action == "archive" {
			return entry.ID
		}
	}
	t.Fatal("the deal's history holds no archive entry")
	return ""
}

func dealVersion(t *testing.T, e *apptest.AppEnv, deal string) int64 {
	t.Helper()
	var read struct {
		Version int64 `json:"version"`
	}
	if status := e.Call(t, "GET", "/v1/deals/"+deal, nil, nil, &read); status != http.StatusOK {
		t.Fatalf("read the deal → %d", status)
	}
	return read.Version
}

type restoreAnswer struct {
	Action     string           `json:"action"`
	LeftBehind []map[string]any `json:"left_behind"`
}

func restoreTheDealArchive(t *testing.T, e *apptest.AppEnv, deal, archive string, version int64) (int, restoreAnswer) {
	t.Helper()
	var answer restoreAnswer
	status := e.Call(t, "POST", fmt.Sprintf("/v1/records/deal/%s/history/%s/restore", deal, archive),
		nil, map[string]string{"If-Match": fmt.Sprint(version)}, &answer)
	return status, answer
}

func countInWorkspace(t *testing.T, e *apptest.AppEnv, query string, args ...any) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(e, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, args...).Scan(&n)
	}); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func dealListShows(t *testing.T, e *apptest.AppEnv, deal string) bool {
	t.Helper()
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/deals?limit=200", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("list deals → %d", status)
	}
	for _, row := range page.Data {
		if row.ID == deal {
			return true
		}
	}
	return false
}

// The round trip through the route the deal page's band presses. The deal, its
// stakeholder and its live list membership come back. The list archived since
// stays retired and is named in the answer. deal.restored goes out.
func TestRestoringAnArchivedDealBringsBackWhatItsArchiveTookDown(t *testing.T) {
	e := listsOnApp(t)
	f := archiveADealOnTwoLists(t, e, "Harbour retrofit")
	if dealListShows(t, e, f.deal) {
		t.Fatal("the deal list shows the archived deal")
	}

	status, answer := restoreTheDealArchive(t, e, f.deal, f.archive, dealVersion(t, e, f.deal))
	if status != http.StatusOK {
		t.Fatalf("restore → %d, want 200", status)
	}
	if answer.Action != "restore" {
		t.Errorf("the answer's action = %q, want restore", answer.Action)
	}
	if len(answer.LeftBehind) != 1 || answer.LeftBehind[0]["kind"] != "list" {
		t.Errorf("left_behind = %+v, want the one membership of the archived list", answer.LeftBehind)
	}
	if !dealListShows(t, e, f.deal) {
		t.Error("the deal list does not show the restored deal")
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM list_member
		WHERE list_id = $1 AND entity_type = 'deal' AND entity_id = $2 AND note = 'from the review'`, f.kept, f.deal); n != 1 {
		t.Errorf("the live list holds the deal %d times with its note, want once", n)
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM list_member WHERE list_id = $1`, f.retired); n != 0 {
		t.Errorf("the list archived since holds %d members, want none: a restore must not reach a retired list", n)
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM relationship WHERE id = $1 AND archived_at IS NULL`, f.stakeholder); n != 1 {
		t.Error("the deal's stakeholder is still archived")
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM event_outbox o JOIN audit_log a
		ON a.id::text = o.envelope#>>'{trace,audit_log_id}'
		WHERE o.envelope->>'type' = 'deal.restored' AND a.entity_id = $1 AND a.action = 'restore'`, f.deal); n != 1 {
		t.Errorf("%d deal.restored events trace to the restore's audit row, want one", n)
	}

	// The archive is undone, so a second press is refused and writes nothing.
	if status, _ := restoreTheDealArchive(t, e, f.deal, f.archive, dealVersion(t, e, f.deal)); status != http.StatusConflict {
		t.Errorf("a second restore → %d, want 409", status)
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'restore'`, f.deal); n != 1 {
		t.Errorf("%d restore rows on the deal, want one", n)
	}
}

// A colleague who neither owns the deal nor holds a grant on it is refused
// with the reason the history shows them. An archive entry of another deal
// answers 404, so the route never says whether it exists.
func TestADealRestoreIsRefusedToAColleagueAndToAnotherDealsEntry(t *testing.T) {
	e := listsOnApp(t)
	f := archiveADealOnTwoLists(t, e, "Quay lighting")
	other := archiveADealOnTwoLists(t, e, "Pier signage")

	e.DescribeCompany(t)
	const email, password = "rep@deal-restore.test", "deal-restore-password"
	rep := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Rhea Rep", "role": "rep"})
	apptest.SetPasswordByLink(t, e, rep, password)
	colleague := e.AsSecondSeat(t, email, password)
	var refusal struct {
		Code string `json:"code"`
	}
	status := colleague.Call(t, "POST", fmt.Sprintf("/v1/records/deal/%s/history/%s/restore", f.deal, f.archive),
		nil, map[string]string{"If-Match": fmt.Sprint(dealVersion(t, e, f.deal))}, &refusal)
	if status != http.StatusConflict || refusal.Code != "not_writable_by_caller" {
		t.Errorf("a colleague's restore → %d %q, want 409 not_writable_by_caller", status, refusal.Code)
	}

	if status, _ := restoreTheDealArchive(t, e, f.deal, other.archive, dealVersion(t, e, f.deal)); status != http.StatusNotFound {
		t.Errorf("a restore naming another deal's archive → %d, want 404", status)
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM deal WHERE id IN ($1, $2) AND archived_at IS NULL`, f.deal, other.deal); n != 0 {
		t.Errorf("%d deals came back from refused restores, want none", n)
	}
}

// A rep may archive a deal they own with edit rights alone. An archive and
// its undo answer to one authority, so they may bring it back the same way.
func TestARepRestoresTheDealTheyArchived(t *testing.T) {
	e := listsOnApp(t)
	e.DescribeCompany(t)
	const email, password = "owner@deal-restore.test", "deal-restore-password"
	rep := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": "Otto Owner", "role": "rep"})
	apptest.SetPasswordByLink(t, e, rep, password)
	owner := e.AsSecondSeat(t, email, password)
	stages := apptest.DiscoverSeededPipeline(t, owner)
	deal := createdID(t, owner, "/v1/deals", AnyMap{
		"name": "Own rollout", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
	})
	if status := owner.Call(t, "DELETE", "/v1/deals/"+deal, nil, nil, nil); status != http.StatusOK && status != http.StatusNoContent {
		t.Fatalf("the owner's archive → %d", status)
	}
	archive := theArchiveEntry(t, owner, deal)
	if entry := theEntryByID(t, readHistory(t, owner, "deal", deal), archive); !entry.Undoable.Undoable {
		t.Errorf("the owner's history offers no undo of their own archive: %s", reasonOf(entry))
	}
	if status, _ := restoreTheDealArchive(t, owner, deal, archive, dealVersion(t, owner, deal)); status != http.StatusOK {
		t.Fatalf("the owner's restore → %d, want 200", status)
	}
	if n := countInWorkspace(t, e, `SELECT count(*) FROM deal WHERE id = $1 AND archived_at IS NULL`, deal); n != 1 {
		t.Error("the deal is still archived")
	}
}
