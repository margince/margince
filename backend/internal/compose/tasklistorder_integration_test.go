// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// One task list, two pages. A rep opens the same two promises from the
// contact and from the account, and they must be in the same order: a list
// that reorders itself depending on which record you came in through teaches
// the reader that neither order means anything.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/org360"
	"github.com/margince/margince/backend/internal/compose/person360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBothPagesRankTwoPromisesTheSameWay(t *testing.T) {
	e := integration.Setup(t)
	person := seedLinkedPerson(t, e, "beide@kunde.example")
	org := seedEmployerOf(t, e, person, "Kunde GmbH")
	due := time.Now().Add(24 * time.Hour).Truncate(time.Microsecond)

	// Both due the same day, so only the tie-break separates them. The one
	// filed FIRST has waited longest and leads on both pages.
	older := logTaskDueAt(t, e, person, "Send the signed contract", due, time.Now().Add(-48*time.Hour))
	logTaskDueAt(t, e, person, "Book the workshop room", due, time.Now().Add(-2*time.Hour))

	personSvc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), time.Now)
	orgSvc := org360.NewService(e.Pool, e.People, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), time.Now)

	personPage, err := personSvc.Assemble(e.Admin(), ids.From[ids.PersonKind](person))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	orgPage, err := orgSvc.Assemble(e.Admin(), ids.From[ids.OrganizationKind](org))
	if err != nil {
		t.Fatalf("assembling the account: %v", err)
	}
	if personPage.NextSteps == nil || len(personPage.NextSteps.Data) != 2 {
		t.Fatalf("the contact carries %v tasks, want the two just written", personPage.NextSteps)
	}
	if orgPage.NextSteps == nil || len(orgPage.NextSteps.Data) != 2 {
		t.Fatalf("the account carries %v tasks, want the two just written", orgPage.NextSteps)
	}

	if got := personPage.NextSteps.Data[0].Id; ids.UUID(got) != older {
		t.Errorf("the contact leads with %v, want the promise filed first %v", got, older)
	}
	if got := orgPage.NextSteps.Data[0].ActivityId; ids.UUID(got) != older {
		t.Errorf("the account leads with %v, want the same promise the contact leads with (%v) — "+
			"two lists of one task set that disagree teach the reader that neither order means anything",
			got, older)
	}
}

// seedEmployerOf gives a person an employer, so the account's any-link task
// read reaches the tasks filed against them.
func seedEmployerOf(t *testing.T, e *integration.Env, person ids.UUID, name string) ids.UUID {
	t.Helper()
	orgID := ids.NewV7()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `
			INSERT INTO organization (id, owner_id, display_name, name_source, source, captured_by)
			VALUES ($1, $2, $3, 'domain', 'connector:gmail', 'connector:gmail')`,
			orgID, e.Rep1, name); err != nil {
			return err
		}
		// An employment is a relationship row, which is the arm the account's
		// task read walks (activities.OrgLinkedActivityExists).
		_, err := tx.Exec(context.Background(), `
			INSERT INTO relationship (id, kind, person_id, organization_id, is_current_primary, source, captured_by)
			VALUES ($1, 'employment', $2, $3, true, 'connector:gmail', 'connector:gmail')`,
			ids.NewV7(), person, orgID)
		return err
	}); err != nil {
		t.Fatalf("seeding an employer: %v", err)
	}
	return orgID
}

// logTaskDueAt writes one open task through the real writer, filed against a
// person, with both dates the ordering reads.
func logTaskDueAt(t *testing.T, e *integration.Env, person ids.UUID, subject string, due, occurred time.Time) ids.UUID {
	t.Helper()
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, DueAt: &due, OccurredAt: &occurred, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: person}},
	})
	if err != nil {
		t.Fatalf("logging task %q: %v", subject, err)
	}
	return ids.UUID(row.Id)
}
