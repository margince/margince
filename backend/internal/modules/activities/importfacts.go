// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// sqlFalse is the column a reader gets when no import fact can be true.
const sqlFalse = "false"

// importFactColumns renders both columns for alias. thread_audience_yours is
// true when ThreadAudienceSetter.Decide would act on the row. That needs a
// human or agent caller on a seat that may write, holding activity:update, and
// a row storekit.ThreadImportedByClause admits.
func importFactColumns(ctx context.Context, alias string, arg func(any) int) map[string]string {
	yours := sqlFalse
	if actor, ok := principal.Actor(ctx); ok && actor.HumanOrItsAgent() && actor.UserID != ids.Nil && mayUpdateActivities(ctx) {
		yours = storekit.ThreadImportedByClause(alias, "$"+strconv.Itoa(arg(actor.UserID)))
	}
	return map[string]string{
		mailboxImportedColumn:     importRowExistsSQL(alias+".id") + " AS " + mailboxImportedColumn,
		threadAudienceYoursColumn: yours + " AS " + threadAudienceYoursColumn,
	}
}

// callerHoldsThreadWrite answers whether the thread audience write acts on this
// message for the caller. callerIsSenderSeat also admits a sender who holds no
// import row, which the write answers with 404, so the drawer asks this
// instead.
func callerHoldsThreadWrite(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return false, nil
	}
	var holds bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $1 AND `+storekit.ThreadImportedByClause("a", "$2")+`)`,
		id.UUID, actor.UserID).Scan(&holds); err != nil {
		return false, fmt.Errorf("activities: reading whether the caller holds the thread write: %w", err)
	}
	return holds, nil
}
