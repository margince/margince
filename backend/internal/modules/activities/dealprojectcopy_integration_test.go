// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A transcript filed on a deal of one project and named under another copies
// both onto the meeting it records. The named project lands; the deal's
// project, which would otherwise come first, does not.
func TestARecordedMeetingTakesTheTranscriptsNamedProjectOverTheDeals(t *testing.T) {
	e := setupSend(t)
	ctx := context.Background()
	customer, deal := seedCustomer(t, e), e.seedDeal(t, "ERP licences")
	company, erp, migration := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Acme', 'seed', 'system')`, []any{company}},
		{`INSERT INTO project (id, name, key, company_id, phase, source, captured_by)
			VALUES ($1, 'ERP rollout', 'ERP1', $3, 'delivering', 'manual', 'human:x'),
			       ($2, 'Datacentre migration', 'DCM1', $3, 'delivering', 'manual', 'human:x')`, []any{erp, migration, company}},
		{`INSERT INTO relationship (kind, project_id, company_id, role, source, captured_by)
			VALUES ('project_company', $1, $2, 'customer', 'manual', 'human:x')`, []any{erp, company}},
		{`UPDATE deal SET company_id = $2, project_id = $3 WHERE id = $1`, []any{deal, company, erp}},
	} {
		if _, err := e.owner.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding the account: %v", err)
		}
	}
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	meeting := customerMeeting(t, e, customer, "Quarterly review", clocktest.Now(t).Add(-2*time.Hour), &booked)

	links := fmt.Sprintf(`[{"entity_type": "contact", "entity_id": %q}, {"entity_type": "deal", "entity_id": %q},
		{"entity_type": "project", "entity_id": %q}]`, customer, deal, migration)
	if _, err := transcriptAs(meetingCtx(e), t, e, meeting, links, ""); err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}
	var onMigration, projects int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FILTER (WHERE project_id = $2), count(*)
		FROM activity_link WHERE activity_id = $1 AND entity_type = 'project'`, meeting, migration).Scan(&onMigration, &projects); err != nil {
		t.Fatal(err)
	}
	if onMigration != 1 || projects != 1 {
		t.Errorf("the meeting is on %d projects, %d of them the named one; want only the named one", projects, onMigration)
	}
}
