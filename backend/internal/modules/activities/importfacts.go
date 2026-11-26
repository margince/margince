// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importRowExistsSQL is the one test for "a seat imported this activity". The
// audience write's refusal and the list row's mailbox_imported both use it.
func importRowExistsSQL(activityRef string) string {
	return "EXISTS (SELECT 1 FROM capture_import WHERE activity_id = " + activityRef + ")"
}

// The two capture_import facts a row control needs, as placeholders in
// activityProjection that importFactColumns renders per query.
const (
	mailboxImportedColumn     = "mailbox_imported"
	threadAudienceYoursColumn = "thread_audience_yours"
)

// importFactColumns renders both columns for alias. thread_audience_yours is
// true when the thread audience write finds the row. That needs the caller to
// have imported it, and the row live and not restricted, as
// capture.ThreadActivityIDsTx requires.
func importFactColumns(ctx context.Context, alias string, arg func(any) int) map[string]string {
	yours := "false"
	if actor, ok := principal.Actor(ctx); ok && actor.HumanOrItsAgent() && actor.UserID != ids.Nil {
		yours = fmt.Sprintf(`(%[1]s.thread_key IS NOT NULL AND %[1]s.archived_at IS NULL AND %[1]s.restricted_at IS NULL
 AND EXISTS (SELECT 1 FROM capture_import i WHERE i.activity_id = %[1]s.id AND i.user_id = $%[2]d))`,
			alias, arg(actor.UserID))
	}
	return map[string]string{
		mailboxImportedColumn:     importRowExistsSQL(alias+".id") + " AS " + mailboxImportedColumn,
		threadAudienceYoursColumn: yours + " AS " + threadAudienceYoursColumn,
	}
}
