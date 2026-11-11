// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assignments

import (
	"context"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMemberHandoverMovesResponsibilitiesWithoutChangingTheirHistory(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		duplicate  bool
		wrongScope bool
		writeOnly  bool
	}{
		{name: "move"},
		{name: "receiver already holds this role", duplicate: true},
		{name: "receiver cannot edit the record", wrongScope: true},
		{name: "caller cannot read the record", writeOnly: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			e := setupRoleEnv(t)
			former := e.seedColleague(t, "Former")
			receiverID := e.seedColleague(t, "Receiver")
			owner := receiverID
			if scenario.wrongScope {
				owner = e.admin
			}
			company := e.seedCompany(t, owner, "workspace")
			ctx := e.actingAs(e.admin, companyCRUD, principal.RowScopeAll)
			created, err := e.store.CreateAssignment(ctx, staffing(company, former, e.accountManager(t)))
			if err != nil {
				t.Fatal(err)
			}
			if scenario.duplicate {
				if _, err := e.store.CreateAssignment(ctx, staffing(company, receiverID, e.accountManager(t))); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.owner.Exec(t.Context(), `UPDATE app_user SET status='deactivated' WHERE id=@id`, pgx.StrictNamedArgs{"id": former}); err != nil {
				t.Fatal(err)
			}
			actor, present := principal.Actor(ctx)
			if !present {
				t.Fatal("the fixture has no actor")
			}
			actor.SeatType = principal.SeatFull
			actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
			actor.Permissions.Objects["user_admin"] = companyCRUD
			if scenario.writeOnly {
				grant := actor.Permissions.Objects["company"]
				grant.Read = false
				actor.Permissions.Objects["company"] = grant
			}
			ctx = principal.WithActor(ctx, actor)
			receiver, present := principal.Actor(e.actingAs(receiverID, companyCRUD, principal.RowScopeOwn))
			if !present {
				t.Fatal("the receiving fixture has no actor")
			}
			receiver.SeatType = principal.SeatFull
			var moved int
			if err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
				var err error
				moved, err = TransferMemberAssignmentsTx(ctx, tx, former, receiver)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var assigned ids.UUID
			var archived bool
			var version int64
			if err := e.owner.QueryRow(context.Background(), `SELECT user_id, archived_at IS NOT NULL, version FROM record_assignment WHERE id=@id`,
				pgx.StrictNamedArgs{"id": created.Id}).Scan(&assigned, &archived, &version); err != nil {
				t.Fatal(err)
			}
			refused := scenario.wrongScope || scenario.writeOnly
			if refused {
				if moved != 0 || assigned != former || archived || version != 1 {
					t.Fatalf("refused move changed the responsibility: moved=%d user=%s archived=%v version=%d", moved, assigned, archived, version)
				}
				return
			}
			if moved != 1 || version != 2 || archived != scenario.duplicate {
				t.Fatalf("move=%d version=%d archived=%v", moved, version, archived)
			}
			if scenario.duplicate && assigned != former || !scenario.duplicate && assigned != receiverID {
				t.Fatalf("responsibility is attributed to %s", assigned)
			}
			var active int
			if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM record_assignment WHERE company_id=@company AND archived_at IS NULL AND user_id=@receiver`,
				pgx.StrictNamedArgs{"company": company, "receiver": receiverID}).Scan(&active); err != nil {
				t.Fatal(err)
			}
			if active != 1 {
				t.Fatalf("the receiver holds %d copies of the responsibility", active)
			}
			var events int
			if err := e.owner.QueryRow(t.Context(), `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id'=@id::text AND envelope->>'type'=@event`, pgx.StrictNamedArgs{
				"id": created.Id, "event": map[bool]string{false: "record_assignment.updated", true: "record_assignment.archived"}[scenario.duplicate],
			}).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if events != 1 {
				t.Fatalf("the move emitted %d events", events)
			}
		})
	}
}
