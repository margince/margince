// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A meeting is with a person. A company is not somebody you can meet: it is
// reached through the person who was in the room.
//
// The rule lives in the database rather than in a write path because the write
// path is not the only writer — the MCP tool, a REST caller and the web app all
// insert into activity_link. A check that only the Go code enforced would be
// enforced for one of the three.
//
// These tests drive raw SQL for exactly that reason: they assert the estate
// refuses the shape, not that some particular caller declines to ask.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedActivityAndOrg writes one activity of the given kind and one organization,
// and answers both ids. Nothing links them: each test decides what to attempt.
// meetingWorkspace binds a workspace the same way the other capture
// integration tests do, so the triggers are exercised under real RLS binding
// rather than as a superuser.
func meetingWorkspace(t *testing.T) (context.Context, *database.DB) {
	t.Helper()
	owner, pool := setupCaptureDB(t)
	ctx := context.Background()
	ws := ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatalf("seeding workspace: %v", err)
	}
	return ctx, database.BindTo(pool, ids.From[ids.WorkspaceKind](ws))
}

func seedActivityAndOrg(ctx context.Context, t *testing.T, db *database.DB, kind string) (ids.UUID, ids.UUID) {
	t.Helper()
	activityID, orgID := ids.NewV7(), ids.NewV7()
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, occurred_at, source, captured_by)
			VALUES ($1, $2, now(), 'manual', 'test')`, activityID, kind); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO organization (id, display_name, source, captured_by)
			VALUES ($1, 'Kugellager Test GmbH', 'manual', 'test')`, orgID)
		return err
	}); err != nil {
		t.Fatalf("seed %s: %v", kind, err)
	}
	return activityID, orgID
}

func linkToOrg(ctx context.Context, db *database.DB, activityID, orgID ids.UUID) error {
	return db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, organization_id)
			VALUES ($1, 'organization', $2)`, activityID, orgID)
		return err
	})
}

// TestACompanyCannotBeMetOrCalled is the rule itself.
func TestACompanyCannotBeMetOrCalled(t *testing.T) {
	ctx, db := meetingWorkspace(t)

	for _, kind := range []string{"meeting", "call"} {
		t.Run(kind, func(t *testing.T) {
			activityID, orgID := seedActivityAndOrg(ctx, t, db, kind)

			err := linkToOrg(ctx, db, activityID, orgID)
			if err == nil {
				t.Fatalf("a %s was linked straight to a company; the estate must refuse it", kind)
			}
			// The refusal has to say what to do instead, because a caller that
			// only learns "no" retries the same shape.
			if !strings.Contains(err.Error(), "is with a person") {
				t.Fatalf("refusal does not name the rule: %v", err)
			}
		})
	}
}

// TestANoteAboutACompanyIsStillAllowed holds the other half. The rule is about
// meetings and calls, which are inherently personal; a note or a task is ABOUT
// a record, and filing one on a company is ordinary.
func TestANoteAboutACompanyIsStillAllowed(t *testing.T) {
	ctx, db := meetingWorkspace(t)

	for _, kind := range []string{"note", "task", "email"} {
		t.Run(kind, func(t *testing.T) {
			activityID, orgID := seedActivityAndOrg(ctx, t, db, kind)
			if err := linkToOrg(ctx, db, activityID, orgID); err != nil {
				t.Fatalf("a %s about a company must still be allowed: %v", kind, err)
			}
		})
	}
}

// TestARekindCannotSmuggleACompanyMeetingPast closes the back door. Linking a
// note to a company is legal, so without this the same forbidden state is
// reachable in two steps: link first, then change the kind.
func TestARekindCannotSmuggleACompanyMeetingPast(t *testing.T) {
	ctx, db := meetingWorkspace(t)

	activityID, orgID := seedActivityAndOrg(ctx, t, db, "note")
	if err := linkToOrg(ctx, db, activityID, orgID); err != nil {
		t.Fatalf("seed the legal note link: %v", err)
	}

	err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE activity SET kind = 'meeting' WHERE id = $1`, activityID)
		return err
	})
	if err == nil {
		t.Fatal("a company-linked note became a meeting; the two-step route must be refused too")
	}
	if !strings.Contains(err.Error(), "is with a person") {
		t.Fatalf("refusal does not name the rule: %v", err)
	}
}

// TestAMeetingWithAPersonIsUntouched proves the rule forbids only the thing it
// means to. The ordinary case — a meeting linked to the person who was there —
// must still write.
func TestAMeetingWithAPersonIsUntouched(t *testing.T) {
	ctx, db := meetingWorkspace(t)

	activityID, _ := seedActivityAndOrg(ctx, t, db, "meeting")
	personID, ownerID := ids.NewV7(), ids.NewV7()

	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO app_user (id, email, display_name, status)
			VALUES ($1, $2, 'Owner', 'active')`,
			ownerID, "owner-"+ownerID.String()+"@example.test"); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO person (id, full_name, owner_id, visibility, source, captured_by)
			VALUES ($1, 'Matthias Ortner', $2, 'workspace', 'manual', 'test')`,
			personID, ownerID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)`, activityID, personID)
		return err
	}); err != nil {
		t.Fatalf("a meeting with the person who was there must write: %v", err)
	}
}
