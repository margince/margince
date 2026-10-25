// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The origin reader asks the content gate itself, so it names no mail the
// caller may not read even when a list hands it one.
func TestAMailOriginNamesOnlyAMailTheCallerMayRead(t *testing.T) {
	e := setupPromises(t)
	open, held := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'inbound', 'Signed order', now(), 'seed', 'system'),
		       ($2, 'email', 'inbound', 'Salary talk', now(), 'seed', $3)`,
		open, held, "connector:imap:"+e.other.String())
	e.exec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, held)
	e.exec(t, `INSERT INTO activity_participant (id, activity_id, role, address, display_name)
		VALUES ($1, $2, 'from', 'buyer@origin.test', 'Bea Buyer'), ($3, $4, 'from', 'hr@origin.test', 'HR')`,
		ids.NewV7(), open, ids.NewV7(), held)

	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true}, "contact": {Read: true}, "company": {Read: true}, "deal": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	var origins map[ids.UUID]*crmcontracts.AttachmentOrigin
	err := NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))).tx(ctx, func(tx pgx.Tx) (err error) {
		origins, err = mailOrigins(ctx, tx, []ids.UUID{open, held})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := origins[open]; got == nil || got.Subject == nil || *got.Subject != "Signed order" ||
		got.Sender == nil || got.Sender.Address != "buyer@origin.test" {
		t.Errorf("the readable mail's origin is %+v, want subject Signed order from buyer@origin.test", got)
	}
	if got, named := origins[held]; named {
		t.Errorf("a mail outside the caller's audience is named: %+v", got)
	}
}
