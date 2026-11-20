// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A saved composer draft is read by its author alone, through every door. It
// moves with a handover of the author's work to the colleague who takes over.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type draftHandoverEnv struct {
	admin, leaver, successor     *apptest.AppEnv
	leaverID, successorID, adopt string
}

const draftHandoverPassword = "draft-handover-password"

func setupDraftHandover(t *testing.T) *draftHandoverEnv {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp")}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Draft handover", "admin@drafthandover.test", "Ada Admin")
	// A colleague is invited only once the workspace has described its company.
	e.DescribeCompany(t)
	seat := func(email, name string) (string, *apptest.AppEnv) {
		id := createdID(t, e, "/v1/users", AnyMap{"email": email, "display_name": name, "role": "rep"})
		apptest.SetPasswordByLink(t, e, id, draftHandoverPassword)
		return id, e.AsSecondSeat(t, email, draftHandoverPassword)
	}
	d := &draftHandoverEnv{admin: e}
	d.leaverID, d.leaver = seat("leaver@drafthandover.test", "Lea Leaver")
	d.successorID, d.successor = seat("successor@drafthandover.test", "Sam Successor")
	d.adopt = d.contact(t, "Adopted Buyer", "adopted@drafthandover.test")
	return d
}

// contact is one the leaver owns, so the handover gives it to the successor too.
func (d *draftHandoverEnv) contact(t *testing.T, name, email string) string {
	t.Helper()
	return createdID(t, d.leaver, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": name, "emails": []AnyMap{{"email": email}},
	})
}

func saveContactDraft(t *testing.T, seat *apptest.AppEnv, contact, body string) wireDraft {
	t.Helper()
	var out wireDraft
	if status := seat.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
		"anchor_type": "contact", "anchor_id": contact,
		"to": []string{"buyer@drafthandover.test"}, "subject": "Next steps", "body": body,
	}, nil, &out); status != http.StatusOK {
		t.Fatalf("saving a draft on %s → %d, want 200", contact, status)
	}
	return out
}

func readContactDraft(t *testing.T, seat *apptest.AppEnv, contact string) (int, wireDraft) {
	t.Helper()
	var out wireDraft
	status := seat.Call(t, "GET", "/v1/mail-drafts?anchor_type=contact&anchor_id="+contact, nil, nil, &out)
	return status, out
}

type handoverCounts struct {
	MailDraft int `json:"mail_draft"`
	Contact   int `json:"contact"`
}

func (d *draftHandoverEnv) handOver(t *testing.T, key string) (moved, left handoverCounts) {
	t.Helper()
	var out struct {
		Moved handoverCounts `json:"moved"`
		Left  handoverCounts `json:"left"`
	}
	if status := d.admin.Call(t, "POST", "/v1/users/"+d.leaverID+"/handover",
		AnyMap{"to_user_id": d.successorID}, map[string]string{"Idempotency-Key": key}, &out); status != http.StatusOK {
		t.Fatalf("handing over the leaver's work → %d, want 200", status)
	}
	return out.Moved, out.Left
}

func (d *draftHandoverEnv) draftsWrittenBy(t *testing.T, author string) int {
	t.Helper()
	var n int
	if err := apptest.InWorkspace(d.admin, t, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT count(*) FROM mail_draft WHERE author_id = $1`, ids.MustParse(author)).Scan(&n)
	}); err != nil {
		t.Fatalf("counting drafts: %v", err)
	}
	return n
}

// A colleague and an admin get the 404 a draft nobody saved gets, over the API
// and over the agent's tool. So neither learns that a draft exists.
func TestOnlyItsAuthorReachesASavedDraft(t *testing.T) {
	d := setupDraftHandover(t)
	draft := saveContactDraft(t, d.leaver, d.adopt, "Half a thought")

	for name, seat := range map[string]*apptest.AppEnv{"a colleague": d.successor, "an admin": d.admin} {
		if status, _ := readContactDraft(t, seat, d.adopt); status != http.StatusNotFound {
			t.Errorf("%s reading the leaver's draft → %d, want 404", name, status)
		}
		if status := seat.Call(t, "DELETE", "/v1/mail-drafts/"+draft.ID, nil, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s discarding the leaver's draft → %d, want 404", name, status)
		}
		if status := seat.Call(t, "PUT", "/v1/mail-drafts", AnyMap{
			"anchor_type": "contact", "anchor_id": d.adopt, "body": "taken over",
		}, map[string]string{"If-Match": "1"}, nil); status != http.StatusConflict {
			t.Errorf("%s saving over the leaver's draft → %d, want 409: they hold no draft of their own there", name, status)
		}
	}
	agent := apptest.NewMCPClient(d.admin, apptest.MCPBearerToken(t, d.admin, "admin agent", "read"))
	refusal := agent.CallRefused(t, "read_details", map[string]any{"action": "mail_draft", "anchor_type": "contact", "anchor_id": d.adopt})
	if !strings.Contains(refusal, "not_found") {
		t.Errorf("the admin's agent reading the leaver's draft was refused with %q, want not_found", refusal)
	}

	if status, got := readContactDraft(t, d.leaver, d.adopt); status != http.StatusOK || got.Body != "Half a thought" || got.Version != draft.Version {
		t.Fatalf("the leaver's own draft after the attempts → %d v%d %q, want it untouched", status, got.Version, got.Body)
	}
}

// The successor reads the leaver's drafts as their own and the leaver no longer
// does. Where the successor already had a draft in the same place, theirs stays.
func TestAHandoverMovesTheLeaversDraftsToTheSuccessor(t *testing.T) {
	d := setupDraftHandover(t)
	shared := d.contact(t, "Shared Buyer", "shared@drafthandover.test")
	saveContactDraft(t, d.leaver, d.adopt, "The leaver's words")
	saveContactDraft(t, d.leaver, shared, "The leaver's other words")
	saveContactDraft(t, d.successor, shared, "The successor's own words")

	var preview struct {
		Owned handoverCounts `json:"owned"`
	}
	if status := d.admin.Call(t, "GET", "/v1/users/"+d.leaverID+"/handover", nil, nil, &preview); status != http.StatusOK || preview.Owned.MailDraft != 2 {
		t.Fatalf("the handover preview → %d with %d drafts, want 200 with 2", status, preview.Owned.MailDraft)
	}
	moved, left := d.handOver(t, "draft-handover-1")
	if moved.MailDraft != 1 || left.MailDraft != 0 {
		t.Errorf("handover moved %d and left %d drafts, want 1 moved (one place was the successor's) and 0 left", moved.MailDraft, left.MailDraft)
	}

	if status, got := readContactDraft(t, d.successor, d.adopt); status != http.StatusOK || got.Body != "The leaver's words" {
		t.Errorf("the successor reading the handed-over draft → %d %q, want 200 with the leaver's words", status, got.Body)
	}
	if status, got := readContactDraft(t, d.successor, shared); status != http.StatusOK || got.Body != "The successor's own words" {
		t.Errorf("the successor's own draft after the handover → %d %q, want it kept", status, got.Body)
	}
	if status, _ := readContactDraft(t, d.leaver, d.adopt); status != http.StatusNotFound {
		t.Errorf("the leaver reading a draft they handed over → %d, want 404", status)
	}
	if n := d.draftsWrittenBy(t, d.leaverID); n != 0 {
		t.Errorf("the leaver still holds %d drafts after the handover, want 0", n)
	}
}

func TestAHandoverOfAMemberWithNoDraftsMovesNone(t *testing.T) {
	d := setupDraftHandover(t)
	moved, left := d.handOver(t, "draft-handover-none")
	if moved.MailDraft != 0 || left.MailDraft != 0 || moved.Contact != 1 {
		t.Errorf("handover of a member without drafts moved %d drafts and %d contacts, left %d drafts; want 0, 1, 0",
			moved.MailDraft, moved.Contact, left.MailDraft)
	}
}

// Removing a member who left moves their drafts with the rest of their work,
// so a saved draft is never a reason the removal must wait.
func TestRemovingAMemberWhoLeftMovesTheirDrafts(t *testing.T) {
	d := setupDraftHandover(t)
	saveContactDraft(t, d.leaver, d.adopt, "The leaver's words")
	if status := d.admin.Call(t, "POST", "/v1/users/"+d.leaverID+"/deactivate", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivating the leaver → %d, want 200", status)
	}
	var preview struct {
		Owned    handoverCounts `json:"owned"`
		Blockers []string       `json:"blockers"`
		Token    string         `json:"token"`
	}
	path := "/v1/users/" + d.leaverID + "/retirement-preview?mode=remove&to_user_id=" + d.successorID
	if status := d.admin.Call(t, "GET", path, nil, nil, &preview); status != http.StatusOK {
		t.Fatalf("the removal preview → %d, want 200", status)
	}
	if preview.Owned.MailDraft != 1 || len(preview.Blockers) != 0 {
		t.Fatalf("the removal preview counts %d drafts with blockers %v, want 1 draft to move and no blocker", preview.Owned.MailDraft, preview.Blockers)
	}
	if status := d.admin.Call(t, "POST", "/v1/users/"+d.leaverID+"/remove",
		AnyMap{"to_user_id": d.successorID, "preview_token": preview.Token}, nil, nil); status != http.StatusNoContent {
		t.Fatalf("removing the leaver → %d, want 204", status)
	}
	if status, got := readContactDraft(t, d.successor, d.adopt); status != http.StatusOK || got.Body != "The leaver's words" {
		t.Errorf("the successor reading the removed member's draft → %d %q, want 200 with their words", status, got.Body)
	}
}
