// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/importscopes"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An import takes the company of the connection it came from, a seat's import
// through any of that seat's scoped connections. An import whose mailbox has no
// owner or no company stays unscoped, and a second run changes nothing.
func TestTheImportScopeTaskCopiesTheMailboxCompanyAndGuessesNone(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	branch := ids.NewV7()
	e.WsExec(t, `INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by) VALUES ($1, 'Branch', 'Branch Ltd', 'vn', 'human:x')`, branch)
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label, legal_entity_id)
		VALUES ('gmail', $1, 'connected', 'vault:test', 'rep@scope.test', $2)`, e.Rep1, branch)
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label)
		VALUES ('gmail', $1, 'connected', 'vault:test', 'other@scope.test')`, e.Rep2)

	seat, ownerless, kept := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, id := range []ids.UUID{seat, ownerless, kept} {
		e.WsExec(t, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by) VALUES($1,'email','Hello','2026-10-01T09:00:00Z','gmail','human:x')`, id)
	}
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id) VALUES($1,$2)`, seat, e.Rep1)
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id) VALUES($1,$2)`, ownerless, e.Rep2)
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,legal_entity_id) VALUES($1,$2,(SELECT id FROM legal_entity WHERE is_default))`, kept, e.Rep1)

	for range 2 {
		runCalendarDescriptionTask(ctx, t, e, importscopes.DataTask())
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND legal_entity_id=$2`, seat, branch); got != 1 {
		t.Fatal("the seat's import did not take its mailbox's company")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND legal_entity_id IS NULL`, ownerless); got != 1 {
		t.Fatal("an import whose mailbox has no company was given one")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM capture_import WHERE activity_id=$1 AND legal_entity_id=(SELECT id FROM legal_entity WHERE is_default)`, kept); got != 1 {
		t.Fatal("an import that already named a company was changed")
	}
}
