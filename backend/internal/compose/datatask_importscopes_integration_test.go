// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/importscopes"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	var defaultEntity ids.UUID
	scanWs(t, e, &defaultEntity, `SELECT id FROM legal_entity WHERE is_default`)
	e.WsExec(t, `INSERT INTO legal_entity (id, display_name, legal_name, country, captured_by) VALUES ($1, 'Branch', 'Branch Ltd', 'vn', 'human:x')`, branch)
	for label, entity := range map[string]ids.UUID{"first@scope.test": branch, "second@scope.test": defaultEntity} {
		e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label, legal_entity_id)
			VALUES ('gmail', $1, 'connected', 'vault:test', $2, $3)`, e.Rep1, label, entity)
	}
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, credential_ref, account_label)
		VALUES ('gmail', $1, 'connected', 'vault:test', 'other@scope.test')`, e.Rep2)
	deliveredBy := func(user ids.UUID, label string) []string {
		var mark string
		scanWs(t, e, &mark, `SELECT `+storekit.MailboxMarkSQL("c")+` FROM capture_connection c WHERE user_id=$1 AND account_label=$2`, user, label)
		return []string{mark}
	}

	secondOnly, firstOnly, both, ownerless, kept := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, id := range []ids.UUID{secondOnly, firstOnly, both, ownerless, kept} {
		e.WsExec(t, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by) VALUES($1,'email','Hello','2026-10-01T09:00:00Z','gmail','human:x')`, id)
	}
	first, second := deliveredBy(e.Rep1, "first@scope.test"), deliveredBy(e.Rep1, "second@scope.test")
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,delivered_by) VALUES($1,$2,$3)`, secondOnly, e.Rep1, second)
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,delivered_by) VALUES($1,$2,$3)`, firstOnly, e.Rep1, first)
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,delivered_by) VALUES($1,$2,$3)`, both, e.Rep1, append(first, second...))
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,delivered_by) VALUES($1,$2,$3)`, ownerless, e.Rep2, deliveredBy(e.Rep2, "other@scope.test"))
	e.WsExec(t, `INSERT INTO capture_import(activity_id,user_id,legal_entity_id) VALUES($1,$2,$3)`, kept, e.Rep1, branch)

	for range 2 {
		runCalendarDescriptionTask(ctx, t, e, importscopes.DataTask())
	}
	for name, want := range map[string]struct {
		activity ids.UUID
		entity   *ids.UUID
	}{
		"mail delivered by the second mailbox":           {secondOnly, &defaultEntity},
		"mail delivered by the first mailbox":            {firstOnly, &branch},
		"mail both mailboxes of two companies delivered": {both, nil},
		"mail of a mailbox with no company":              {ownerless, nil},
		"mail that already named a company":              {kept, &branch},
	} {
		var got *ids.UUID
		scanWs(t, e, &got, `SELECT legal_entity_id FROM capture_import WHERE activity_id=$1`, want.activity)
		if (want.entity == nil) != (got == nil) || (got != nil && *got != *want.entity) {
			t.Errorf("%s has company %v, want %v", name, got, want.entity)
		}
	}
}

func scanWs(t *testing.T, e *integration.Env, dest any, sql string, args ...any) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, sql, args...).Scan(dest)
	}); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}
