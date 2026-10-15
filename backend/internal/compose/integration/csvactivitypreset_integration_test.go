// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagePresetActivities uploads one activities CSV and stages it with the
// given preset links, answering the status and the run.
func stagePresetActivities(t *testing.T, e *apptest.AppEnv, body string, presets []AnyMap) (status int, _ importRunDTO) {
	t.Helper()
	profile, uploaded := uploadCSV(t, e, "activity", body)
	if uploaded != http.StatusOK {
		t.Fatalf("upload → %d", uploaded)
	}
	request := AnyMap{
		"connector": "csv", "source_ref": profile.SourceRef, "object": "activity",
		"mapping": AnyMap{"ref": "ref", "type": "type", "subject": "subject", "occurred_at": "occurred_at"},
	}
	if presets != nil {
		request["preset_links"] = presets
	}
	var answer json.RawMessage
	status = testCall(t, e, "/v1/imports", request, &answer)
	var run importRunDTO
	if status == http.StatusAccepted {
		if err := json.Unmarshal(answer, &run); err != nil {
			t.Fatalf("decoding the staged run %s: %v", answer, err)
		}
	}
	return status, run
}

func contactLinkedActivities(t *testing.T, e *apptest.AppEnv, contactID string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT count(*) FROM activity_link l JOIN activity a ON a.id = l.activity_id
		 WHERE l.contact_id = $1 AND a.archived_at IS NULL`, contactID).Scan(&n); err != nil {
		t.Fatalf("counting the contact's activities: %v", err)
	}
	return n
}

func TestAPresetContactLinksEveryRowAndAnInvisibleOneIsNotFound(t *testing.T) {
	e := setupImportApp(t)
	e.DescribeCompany(t)
	colleague := createdID(t, e, "/v1/users", AnyMap{"email": "colleague@act-preset.test", "display_name": "Colleague", "role": "rep"})
	hidden := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Hidden Hana",
		"emails": []AnyMap{{"email": "hana@act-preset.test", "email_type": "work", "is_primary": true}},
	})
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, hidden, colleague); err != nil {
		t.Fatalf("making the contact private to the colleague: %v", err)
	}
	visible := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Visible Vic",
		"emails": []AnyMap{{"email": "vic@act-preset.test", "email_type": "work", "is_primary": true}},
	})
	file := "ref,type,subject,occurred_at\n" +
		"A-1,note,First preset note,2025-03-02\n" +
		"A-2,call,Second preset call,2025-03-03T09:00:00Z\n"

	for name, id := range map[string]string{"an invisible contact": hidden, "an unknown contact": ids.NewV7().String()} {
		status, _ := stagePresetActivities(t, e, file, []AnyMap{{"type": "contact", "id": id}})
		if status != http.StatusNotFound {
			t.Errorf("%s → %d, want 404 with no word of which it is", name, status)
		}
	}

	status, run := stagePresetActivities(t, e, file, []AnyMap{{"type": "contact", "id": visible}})
	if status != http.StatusAccepted {
		t.Fatalf("stage with a visible contact → %d, want 202", status)
	}
	preview := readImportReport(t, e, run.ID)
	approveAndRead(t, e, run.ID)
	committed := readImportReport(t, e, run.ID)
	if committed.Disposition.Created != 2 || committed.Disposition.Skipped != 0 {
		t.Fatalf("commit = %+v, issues %+v, want both rows landed without a links column", committed.Disposition, committed.Issues)
	}
	if preview.Disposition.Created != committed.Disposition.Created || preview.Disposition.Skipped != committed.Disposition.Skipped {
		t.Fatalf("dry run = %+v, commit = %+v, want the same counts", preview.Disposition, committed.Disposition)
	}
	if got := contactLinkedActivities(t, e, visible); got != 2 {
		t.Fatalf("%d activities filed under the contact, want 2", got)
	}
}

func TestAPresetIsRefusedOnARunThatLandsAnythingButActivities(t *testing.T) {
	e := setupImportApp(t)
	contact := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Preset Pat",
		"emails": []AnyMap{{"email": "pat@act-preset-only.test", "email_type": "work", "is_primary": true}},
	})
	profile, status := uploadCSV(t, e, "contact", "ref,email,name\nP-1,x@act-preset-only.test,X\n")
	if status != http.StatusOK {
		t.Fatalf("upload → %d", status)
	}
	got := testCall(t, e, "/v1/imports", AnyMap{
		"connector": "csv", "source_ref": profile.SourceRef, "object": "contact",
		"mapping":      AnyMap{"ref": "ref", "email": "email", "name": "full_name"},
		"preset_links": []AnyMap{{"type": "contact", "id": contact}},
	}, nil)
	if got != http.StatusUnprocessableEntity {
		t.Fatalf("a preset on a contacts file → %d, want %d", got, http.StatusUnprocessableEntity)
	}
}

func TestAPresetSavedOnADraftIsTheOneTheCommitFilesUnder(t *testing.T) {
	e := setupImportApp(t)
	contact := createdID(t, e, "/v1/contacts", AnyMap{
		"source": "manual", "full_name": "Draft Dana",
		"emails": []AnyMap{{"email": "dana@act-preset-draft.test", "email_type": "work", "is_primary": true}},
	})
	profile, status := uploadCSV(t, e, "activity", "ref,type,subject,occurred_at\nA-1,note,Saved on a draft,2025-03-02\n")
	if status != http.StatusOK {
		t.Fatalf("upload → %d", status)
	}
	var saved importRunDTO
	if got := e.Call(t, http.MethodPatch, "/v1/imports/"+profile.RunID, AnyMap{
		"version": 1, "object": "activity", "mapping": AnyMap{"ref": "ref", "type": "type", "subject": "subject", "occurred_at": "occurred_at"},
		"preset_links": []AnyMap{{"type": "contact", "id": contact}},
	}, nil, &saved); got != http.StatusOK {
		t.Fatalf("PATCH → %d, want 200", got)
	}
	if got := testCall(t, e, "/v1/imports/"+saved.ID+"/validate", AnyMap{"version": saved.Version}, nil); got != http.StatusAccepted {
		t.Fatalf("validate → %d, want 202", got)
	}
	approveAndRead(t, e, saved.ID)
	if got := contactLinkedActivities(t, e, contact); got != 1 {
		t.Fatalf("%d activities filed under the contact, want the one the draft's preset names", got)
	}
}
