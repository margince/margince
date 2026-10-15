// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// activityEventsOf reads the envelopes staged for one activity, by type.
func activityEventsOf(t *testing.T, e *apptest.AppEnv, id string) map[string]events.Envelope {
	t.Helper()
	rows, err := e.Owner.Query(context.Background(),
		`SELECT envelope FROM event_outbox WHERE envelope #>> '{entity,id}' = $1 ORDER BY id`, id)
	if err != nil {
		t.Fatalf("reading the activity's events: %v", err)
	}
	defer rows.Close()
	out := map[string]events.Envelope{}
	for rows.Next() {
		var env events.Envelope
		if err := rows.Scan(&env); err != nil {
			t.Fatalf("reading an event: %v", err)
		}
		out[env.Type] = env
	}
	return out
}

func TestAnImportedActivityStartsNoAutomation(t *testing.T) {
	e := setupImportApp(t)
	run, _ := importActivities(t, e, map[string]string{
		"contacts.csv":   "ref,email,name\nP-1,jo@act-marked.example,Jo\n",
		"activities.csv": activityHeader + "\nA-1,meeting,Marked meeting,,2025-03-02,contact:P-1,30,held,\n",
	}, contactAndActivityParts())
	imported := activityNamed(t, e, "Marked meeting")
	captured, ok := activityEventsOf(t, e, imported.ID)["activity.captured"]
	if !ok {
		t.Fatal("no activity.captured was staged for the imported meeting")
	}
	if !events.IsImported(captured) {
		t.Errorf("activity.captured of the imported meeting does not name run %s: %s", run.ID, captured.Payload)
	}
	for _, group := range []string{"cg:workflows", "cg:flow-bridge", "cg:capture-enrich", "cg:intro-advance", "cg:notice-case-open", "cg:commitment-settle", "cg:vcard-ingest"} {
		if !events.SkipsImported(group) {
			t.Errorf("%s does not skip imported events", group)
		}
	}

	liveID := createdID(t, e, "/v1/activities", AnyMap{"kind": "note", "subject": "Live note", "source": "manual"})
	live, ok := activityEventsOf(t, e, liveID)["activity.captured"]
	if !ok || events.IsImported(live) {
		t.Errorf("a note a rep logged = %+v (staged %v), want an activity.captured that names no import", live.Payload, ok)
	}
}

func TestAnImportedActivityEventCarriesTheRunId(t *testing.T) {
	e := setupImportApp(t)
	run, _ := importActivities(t, e, map[string]string{
		"contacts.csv":   "ref,email,name\nP-1,jo@act-runid.example,Jo\n",
		"activities.csv": activityHeader + "\nA-1,note,Run id note,,2025-03-02,contact:P-1,,,\n",
	}, contactAndActivityParts())
	id := activityNamed(t, e, "Run id note").ID
	var runID string
	if err := e.Owner.QueryRow(context.Background(), `
		SELECT envelope -> 'payload' ->> 'import_run_id' FROM event_outbox
		 WHERE envelope #>> '{entity,id}' = $1 AND envelope ->> 'type' = 'activity.captured'`, id).Scan(&runID); err != nil {
		t.Fatalf("reading the event: %v", err)
	}
	if runID != run.ID {
		t.Errorf("import_run_id = %q, want %q", runID, run.ID)
	}
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	if archived, ok := activityEventsOf(t, e, id)["activity.archived"]; !ok || !events.IsImported(archived) {
		t.Errorf("activity.archived written by the undo = %s (staged %v), want it to name the run", archived.Payload, ok)
	}
}

func TestUndoArchivesActivitiesFirst(t *testing.T) {
	e := setupImportApp(t)
	pipeline, open, _, _ := seededStages(t, e)
	run, _ := importActivities(t, e, map[string]string{
		"companies.csv":  "ref,name\nCO-1,Undo Activities GmbH\n",
		"contacts.csv":   "ref,email,name\nP-1,jo@undo-acts.example,Jo\n",
		"deals.csv":      fmt.Sprintf("ref,name,pipeline,stage\nD-1,Undo deal,%s,%s\n", pipeline, open),
		"activities.csv": activityHeader + "\nA-1,note,Undo note,,2025-03-02,deal:D-1; contact:P-1,,,\n",
	}, append(append(bundleParts(), dealImportParts("ref,name,pipeline,stage")...), activityImportParts()...))
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	order := archiveOrder(t, e, run.ID, "activity", "deal", "contact", "company")
	if len(order) != 4 || order[0] != "activity" || order[1] != "deal" {
		t.Fatalf("archive order %v, want the activity, then the deal, the contact and the company", order)
	}
}

func TestUndoKeepsAnActivityEditedSinceTheImport(t *testing.T) {
	e := setupImportApp(t)
	run, _ := importActivities(t, e, map[string]string{
		"contacts.csv": "ref,email,name\nP-1,jo@undo-kept.example,Jo\n",
		"activities.csv": activityHeader + "\n" +
			"A-1,note,Edited by a rep,,2025-03-02,contact:P-1,,,\n" +
			"A-2,note,Left alone,,2025-03-03,contact:P-1,,,\n",
	}, contactAndActivityParts())
	edited := activityNamed(t, e, "Edited by a rep")
	if status := e.Call(t, http.MethodPatch, "/v1/activities/"+edited.ID, AnyMap{"body": "Called them back"}, nil, nil); status != http.StatusOK {
		t.Fatalf("editing the activity → %d", status)
	}
	var report importReportWithUndoDTO
	if status := importCall(t, e, run.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	e.Call(t, http.MethodGet, "/v1/imports/"+run.ID+"/report", nil, nil, &report)
	if report.Undo == nil || len(report.Undo.Kept) != 1 || report.Undo.Kept[0].ID != edited.ID {
		t.Fatalf("undo = %+v, want the edited activity kept", report.Undo)
	}
	if liveActivities(t, e, "Edited by a rep") != 1 || liveActivities(t, e, "Left alone") != 0 {
		t.Fatal("want the edited activity still live and the untouched one archived")
	}
}

func TestTheActivityArchiveReAsksUntouchedInsideItsTransaction(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	subject := "Imported note"
	made, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{Kind: "note", Subject: &subject, Source: "test"})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(made.Id))
	imported := seedInstant(t)

	body := "A rep called them"
	if _, err := e.Activities.UpdateActivity(admin, id, activities.UpdateActivityInput{Body: &body}); err != nil {
		t.Fatalf("the human edit: %v", err)
	}
	var touched *activities.HumanTouchedError
	if _, err := e.Activities.ArchiveActivity(admin, id, nil, activities.NotTouchedByHumanSince(imported)); !errors.As(err, &touched) {
		t.Fatalf("archiving an activity a rep edited → %v, want the touched refusal", err)
	}
	if !errors.Is(touched, apperrors.ErrConflict) {
		t.Errorf("the refusal = %v, want a conflict so an undo records the activity as kept", touched)
	}
	kept, err := e.Activities.GetActivity(admin, id, storekit.LiveOnly)
	if err != nil || kept.ArchivedAt != nil {
		t.Fatalf("activity after the refusal = %+v, %v, want it live", kept, err)
	}
	if _, err := e.Activities.ArchiveActivity(admin, id, nil); err != nil {
		t.Errorf("archiving with no precondition → %v, want the behaviour every other caller has", err)
	}
}

// A re-import rewrites an activity under the approving user's own name, and
// that write is the import's, so undoing the first run still archives the row.
func TestUndoArchivesAnActivityALaterImportRewrote(t *testing.T) {
	e := setupImportApp(t)
	files := func(subject string) map[string]string {
		return map[string]string{
			"contacts.csv":   "ref,email,name\nP-1,jo@undo-rewrite.example,Jo\n",
			"activities.csv": activityHeader + "\n" + fmt.Sprintf("A-1,note,%s,,2025-03-02,contact:P-1,,,\n", subject),
		}
	}
	parts := contactAndActivityParts()
	first, _ := importActivities(t, e, files("Rewrite me"), parts)
	_, second := importActivities(t, e, files("Rewritten by the file"), parts)
	if second.Disposition.Updated != 1 {
		t.Fatalf("re-import = %+v, want the note updated", second.Disposition)
	}
	if status := importCall(t, e, first.ID, "undo", nil, nil); status != http.StatusAccepted {
		t.Fatalf("undo → %d", status)
	}
	if liveActivities(t, e, "Rewritten by the file") != 0 {
		t.Fatal("want the activity archived: the rewrite was the import's, not a colleague's")
	}
}
