// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package toolrole

import (
	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func seedHandoff(s sweepSeed) string {
	s.t.Helper()
	lead, err := ids.Parse(s.create["lead"]())
	if err != nil {
		s.t.Fatal(err)
	}
	actor, err := ids.Parse(apptest.SessionUserID(s.t, s.e))
	if err != nil {
		s.t.Fatal(err)
	}
	var workspace ids.UUID
	if err := s.e.Owner.QueryRow(s.t.Context(), `SELECT id FROM workspace`).Scan(&workspace); err != nil {
		s.t.Fatal(err)
	}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(s.t.Context(), workspace), ids.NewV7())
	db := database.BindTo(s.e.Pool, ids.From[ids.WorkspaceKind](workspace))
	err = db.Tx(ctx, func(tx pgx.Tx) error {
		members, err := identity.MemberPrincipalsTx(ctx, tx, []ids.UUID{actor})
		if err != nil {
			return err
		}
		member, ok := members[actor]
		if !ok {
			s.t.Fatal("the signed-in fixture member is not active")
		}
		ctx = principal.WithActor(ctx, member)
		return nil
	})
	if err != nil {
		s.t.Fatal(err)
	}
	handoff, err := contacts.NewStore(db).SubmitHandoff(ctx, contacts.NewSDRHandoff{LeadID: &lead, AssignedTo: &actor})
	if err != nil {
		s.t.Fatal(err)
	}
	s.pool.note(handoff.String(), "receiver", actor.String())
	return handoff.String()
}
