// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var requestInstant = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func seedEmailRequest(t *testing.T, e *loadEnv, subject, label, verdict string) ids.UUID {
	t.Helper()
	store := storeKnowing(e)
	contact := e.buyer(t)
	direction, key := "inbound", ids.NewV7().String()
	body := "From: buyer@customer.test\nTo: rep-" + e.rep.String() + "@load.test\n\nPlease send the report."
	source, _, err := store.LogActivity(asClassifier(e), LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: &direction,
		ThreadKey: key, Source: "test", OccurredAt: &requestInstant,
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(source.Id)
	// The importing mailbox is the connector boundary; source and task use their real writers.
	e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, id, e.rep)
	e.exec(t, `INSERT INTO activity_participant (activity_id, role, address) VALUES ($1, 'to', $2)`, id, "rep-"+e.rep.String()+"@load.test")
	if _, err := store.SetCaptureLabel(asClassifier(e), id, label); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetOwedVerdict(asClassifier(e), id, verdict); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestEmailRequestCreatesOneUndatedPersonalTaskWithSourceEvidence(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Product report", "commitment", OwedVerdictAsksUs)
	seedEmailRequest(t, e, "Thanks for the report", "noise", OwedVerdictAsksUs)
	seedEmailRequest(t, e, "I will send my draft", "commitment", OwedVerdictInformsUs)
	store := storeKnowing(e)
	for range 2 {
		if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	var task ids.UUID
	var count int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*), min(id::text)::uuid FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&count, &task); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("request produced %d tasks", count)
	}
	got, err := store.GetActivity(e.as(), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.DueAt != nil || got.AssigneeId == nil || ids.UUID(*got.AssigneeId) != e.rep || got.SourceActivityId == nil || ids.UUID(*got.SourceActivityId) != source {
		t.Fatalf("request lost responsibility or evidence, or invented a date: %+v", got)
	}
	if got.Audience == nil || *got.Audience != crmcontracts.ActivityAudienceSelected {
		t.Fatal("request task is not personal")
	}
	var audience string
	if err := e.owner.QueryRow(e.as(), `SELECT after->>'audience' FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'create'`, task).Scan(&audience); err != nil {
		t.Fatal(err)
	}
	if audience != "selected" {
		t.Fatal("creation audit failed to record the personal audience")
	}
	done := true
	assignee, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing assignee")
	}
	assignee.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	assignee.Permissions.RowScope = principal.RowScopeOwn
	assignee.Permissions.RoleKeys = nil
	if _, err := store.UpdateActivity(principal.WithActor(e.as(), assignee), ids.From[ids.ActivityKind](task), UpdateActivityInput{IsDone: &done}); err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	moves, err := store.EmailSummariesByID(e.as(), []ids.UUID{source})
	if err != nil {
		t.Fatal(err)
	}
	if moves[source].Move != crmcontracts.EmailSummaryMoveNone {
		t.Fatal("completed request still asks for a reply")
	}
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("completed, informational or conflicting verdicts created extra tasks: %d", count)
	}
	// A colleague may discover the linked task, but cannot read the source-derived title.
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.UserID, actor.ID = e.other, "human:"+e.other.String()
	other, err := store.GetActivity(principal.WithActor(e.as(), actor), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if other.Subject != nil {
		t.Fatal("colleague read a personal request")
	}
	// Removing access to the evidence also withholds the derived reminder.
	e.exec(t, `UPDATE activity SET archived_at = $2 WHERE id = $1`, source, requestInstant)
	got, err = store.GetActivity(e.as(), ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != nil {
		t.Fatal("archived evidence remained readable through its task")
	}
}

func TestEmailMoveUsesTheConversationAndActionEvidence(t *testing.T) {
	e := setupLoad(t)
	first := seedEmailRequest(t, e, "Send the report", "commitment", OwedVerdictAsksUs)
	answered := seedEmailRequest(t, e, "Competitor question", "commitment", OwedVerdictAsksUs)
	info := seedEmailRequest(t, e, "I will follow up", "noise", OwedVerdictInformsUs)
	store := storeKnowing(e)
	original, err := store.GetActivity(e.as(), ids.From[ids.ActivityKind](answered), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	at, direction := requestInstant.Add(time.Minute), "outbound"
	if _, _, err := store.LogActivity(asClassifier(e), LogActivityInput{Kind: "email", Source: "test", ThreadKey: *original.ThreadKey, Direction: &direction, OccurredAt: &at}); err != nil {
		t.Fatal(err)
	}
	got, err := store.EmailSummariesByID(e.as(), []ids.UUID{first, answered, info})
	if err != nil {
		t.Fatal(err)
	}
	if got[first].Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("unrelated reply cleared the report request")
	}
	if got[answered].Move != crmcontracts.EmailSummaryMoveNone || got[info].Move != crmcontracts.EmailSummaryMoveNone {
		t.Fatal("answered or informational email still claimed a reply")
	}
	// Both positive and negative obligations agree on the list and the detail.
	for _, id := range []ids.UUID{first, answered, info} {
		detail, err := store.GetEmailPresentation(e.as(), ids.From[ids.ActivityKind](id), nil)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Summary.Move != got[id].Move {
			t.Fatal("detail disagreed with the list")
		}
	}
}

func TestArchivingAnUnfinishedEmailTaskRestoresTheRequestWithoutRecapture(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Send the report", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var task ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&task); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArchiveActivity(asClassifier(e), ids.From[ids.ActivityKind](task), nil); err != nil {
		t.Fatal(err)
	}
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	moves, err := store.EmailSummariesByID(e.as(), []ids.UUID{source})
	if err != nil {
		t.Fatal(err)
	}
	if moves[source].Move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatal("archiving an unfinished reminder settled the request")
	}
	var count int
	if err := e.owner.QueryRow(e.as(), `SELECT count(*) FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("archived reminder was recaptured")
	}
}

func TestReassignedEmailReminderRequiresReadableSource(t *testing.T) {
	e := setupLoad(t)
	source := seedEmailRequest(t, e, "Send the report", "commitment", OwedVerdictAsksUs)
	store := storeKnowing(e)
	if err := store.CaptureEmailRequests(asClassifier(e), requestInstant.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var task ids.UUID
	if err := e.owner.QueryRow(e.as(), `SELECT id FROM activity WHERE source_system = 'email_request' AND source_activity_id = $1`, source).Scan(&task); err != nil {
		t.Fatal(err)
	}
	manager, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing manager")
	}
	manager.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Update: true}
	receiver := ids.From[ids.UserKind](e.other)
	if _, err := store.UpdateActivity(principal.WithActor(e.as(), manager), ids.From[ids.ActivityKind](task), UpdateActivityInput{AssigneeID: &receiver}); err != nil {
		t.Fatal(err)
	}
	manager.UserID, manager.ID = e.other, "human:"+e.other.String()
	recipient := principal.WithActor(e.as(), manager)
	got, err := store.GetActivity(recipient, ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject == nil || *got.Subject != "Send the report" {
		t.Fatal("new assignee cannot read the reminder")
	}
	e.exec(t, `UPDATE activity SET archived_at = $2 WHERE id = $1`, source, requestInstant)
	got, err = store.GetActivity(recipient, ids.From[ids.ActivityKind](task), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Subject != nil {
		t.Fatal("reassignment granted access to revoked source evidence")
	}
}
