// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestEveryRepairableTaskFamilyIsFoundAndReconciledThroughItsOwner(t *testing.T) {
	fixtures := map[string]func(*testing.T) legacyRepairFixture{
		provenance.LeadRepliedSource:    func(t *testing.T) legacyRepairFixture { return seedLegacyRepair(t, false) },
		provenance.LeadForContactSource: seedLegacyContactArrivalRepair,
		provenance.WhichDealSource:      seedLegacyDealQuestionRepair,
	}
	tested := 0
	for _, source := range storekit.TaskSourceFamilies() {
		supported, _ := storekit.TaskRepairPolicy(source)
		if !supported {
			continue
		}
		seed, ok := fixtures[source]
		if !ok {
			t.Fatalf("repair supports %s without an owner integration fixture", source)
		}
		tested++
		t.Run(source, func(t *testing.T) {
			f := seed(t)
			review, item := f.preview(t)
			expected := repairNormalizeTask
			if source == provenance.WhichDealSource {
				expected = repairLeaveCurrent
			}
			if item.Action != expected {
				t.Fatalf("family %s: action %s, want %s", source, item.Action, expected)
			}
			switch expected {
			case repairNormalizeTask:
				if !f.applyReviewed(t, review, item) {
					t.Fatal("owner correction was not applied")
				}
				if f.e.WsCount(t, "SELECT count(*) FROM activity WHERE id=@id AND due_at IS NULL AND NOT is_done AND archived_at IS NULL", pgx.NamedArgs{"id": f.task}) != 1 {
					t.Fatal("family correction changed the work or kept its generated deadline")
				}
			default:
				if _, err := f.e.Activities.ArchiveActivity(f.e.Admin(), ids.From[ids.ActivityKind](f.source), nil); err != nil {
					t.Fatal(err)
				}
				_, after := f.preview(t)
				if after.Action != repairArchiveReminder {
					t.Fatalf("obsolete question was kept as current: %+v", after)
				}
			}
		})
	}
	if tested != len(fixtures) {
		t.Fatal("repair fixtures no longer match the production registry")
	}
	if supported, requiresSource := storekit.TaskRepairPolicy("unknown-family"); supported || requiresSource {
		t.Fatal("an unknown family acquired repair authority")
	}
}

func seedLegacyContactArrivalRepair(t *testing.T) legacyRepairFixture {
	t.Helper()
	e := integration.Setup(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	owner := ids.From[ids.UserKind](e.Rep2)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Demo contact", Source: "manual", OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	writer := newLeadWorkTasks(InstallationDB(e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	in := writer.arrivalTaskInput(contacts.LeadArrival{ContactID: ids.From[ids.ContactKind](ids.UUID(contact.Id)), OwnerID: e.Rep2, Key: "legacy-demo-arrival"}, "Review an arrival", "An arrival for an existing contact.")
	in.DueAt = &now
	task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), "system:lead-for-contact"), in)
	if err != nil {
		t.Fatal(err)
	}
	return legacyRepairFixture{e: e, now: now, task: ids.From[ids.ActivityKind](ids.UUID(task.Id))}
}

func seedLegacyDealQuestionRepair(t *testing.T) legacyRepairFixture {
	t.Helper()
	e := integration.Setup(t)
	now := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: new("Scope enquiry"), Direction: new("inbound"), Source: "manual", OccurredAt: &now})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	owner := ids.From[ids.UserKind](e.Rep2)
	family, key := provenance.WhichDealSource, "legacy-demo-question"
	task, _, err := e.Activities.LogActivity(principal.SystemActing(e.Admin(), whichDealActor), activities.LogActivityInput{
		Kind: "task", Subject: new("Which deal holds this enquiry?"), Source: family, SourceSystem: &family, SourceID: &key, SourceActivityID: &sourceID,
		AssigneeID: &owner, Origin: activities.OriginSystemRemediation, OccurredAt: &now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return legacyRepairFixture{e: e, now: now, task: ids.From[ids.ActivityKind](ids.UUID(task.Id)), source: sourceID}
}
