// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestUnlinkedMeetingNotesLoseSubjectMentionsWithoutChangingTheInvitation(t *testing.T) {
	env := setupSARIdentifiers(t)
	ordinary, held, company := ids.NewV7(), ids.NewV7(), ids.NewV7()
	ctx := context.Background()
	mustExec(ctx, t, env.owner, `INSERT INTO company(id,display_name,legal_hold,source,captured_by) VALUES($1,'Held account',true,'manual','system:test')`, company)
	for _, id := range []ids.UUID{ordinary, held} {
		mustExec(ctx, t, env.owner, `INSERT INTO activity(id,kind,subject,body,meeting_notes,source,source_system,captured_by,meeting_status)
 VALUES($1,'meeting','Calendar review','Provider agenda','Anna Example approved the renewal','gcal','gcal','connector:gcal','held')`, id)
	}
	mustExec(ctx, t, env.owner, `INSERT INTO activity_link(activity_id,entity_type,company_id) VALUES($1,'company',$2)`, held, company)
	var changed []ids.UUID
	if err := env.db.Tx(env.ctx, func(tx pgx.Tx) error {
		var err error
		changed, err = redactUnlinkedNotes(env.ctx, tx, subjectKeys{displayName: "Anna Example"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != ordinary {
		t.Fatalf("changed=%v, want only the unheld meeting", changed)
	}
	for _, id := range []ids.UUID{ordinary, held} {
		var body, notes string
		if err := env.owner.QueryRow(ctx, `SELECT body,meeting_notes FROM activity WHERE id=$1`, id).Scan(&body, &notes); err != nil {
			t.Fatal(err)
		}
		if body != "Provider agenda" {
			t.Fatalf("invitation text changed: %q", body)
		}
		if strings.Contains(notes, "Anna Example") != (id == held) {
			t.Fatalf("wrong note redaction for %v: %q", id, notes)
		}
	}
}
