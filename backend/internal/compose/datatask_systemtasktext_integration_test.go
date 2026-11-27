// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/qualifytasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func TestLegacySystemTaskTextRepairIsBoundedAtomicAndPreservesUserWording(t *testing.T) {
	for _, mode := range []string{"commit", "rollback"} {
		t.Run(mode, func(t *testing.T) {
			e := integration.Setup(t)
			system := principal.SystemActing(e.Admin(), "system:legacy-text")
			var untouched []ids.ActivityID
			for _, editor := range []string{"none", "none", "human", "agent", "human body", "agent body"} {
				source, key := provenance.LeadSLATaskSource, ids.NewV7().String()
				row, _, err := e.Activities.LogActivity(system, activities.LogActivityInput{
					Kind: "task", Subject: new(systemtext.Literal("automation.new_lead")), Source: "system",
					SourceSystem: &source, SourceID: &key, Origin: activities.OriginSystemRemediation,
				})
				if err != nil {
					t.Fatal(err)
				}
				id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
				if editor == "none" {
					untouched = append(untouched, id)
					continue
				}
				ctx := e.Admin()
				if editor == "agent" || editor == "agent body" {
					actor, ok := principal.Actor(ctx)
					if !ok {
						t.Fatal("admin principal missing")
					}
					actor.Type, actor.ID = principal.PrincipalAgent, "agent:legacy-text"
					actor.OnBehalfOf = e.AdminUser
					actor.Scopes = principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite)
					ctx = principal.WithActor(ctx, actor)
				}
				update := activities.UpdateActivityInput{Subject: new(systemtext.Literal("task.reply_to_email"))}
				if editor == "human body" || editor == "agent body" {
					update.Body, update.Subject = update.Subject, nil
				}
				if _, err := e.Activities.UpdateActivity(ctx, id, update); err != nil {
					t.Fatal(err)
				}
			}
			task := qualifytasks.SystemTextDataTask()
			task.Steps[0].Limit, task.Steps[0].MinLimit = 1, 1
			ctx := datatask.Acting(e.Admin(), task.ID)
			original := task.Steps[0].Batch
			failAfterWrite := func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				out, err := original(ctx, tx, in)
				if err != nil {
					return out, err
				}
				if out.Changed != 1 {
					t.Fatalf("bounded batch changed %d", out.Changed)
				}
				return out, errors.New("stop after descriptor writes")
			}
			if mode == "rollback" {
				before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`)
				events := e.WsCount(t, `SELECT count(*) FROM event_outbox`)
				task.Steps[0].Batch = failAfterWrite
				runMachineAddressTask(ctx, t, e, task)
				if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE system_text IS NOT NULL AND source_system=@source`, pgx.NamedArgs{"source": provenance.LeadSLATaskSource}); n != 0 {
					t.Fatal("descriptor escaped executor rollback")
				}
				if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`) != before || e.WsCount(t, `SELECT count(*) FROM event_outbox`) != events {
					t.Fatal("repair audit or event escaped executor rollback")
				}
				if err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
					_, err := original(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte(`"invalid"`)})
					return err
				}); err == nil {
					t.Fatal("repair accepted a malformed cursor")
				}
				return
			}
			runMachineAddressTask(ctx, t, e, task)
			if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE system_text IS NOT NULL AND source_system=@source`, pgx.NamedArgs{"source": provenance.LeadSLATaskSource}); n != len(untouched) {
				t.Fatalf("described %d tasks, want only %d untouched tasks", n, len(untouched))
			}
			for _, id := range untouched {
				if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=@id AND system_text IS NOT NULL`, pgx.NamedArgs{"id": id}); n != 1 {
					t.Fatal("untouched task was not described")
				}
			}
			before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`)
			runMachineAddressTask(ctx, t, e, task)
			if e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`) != before {
				t.Fatal("repair replay wrote extra audit")
			}
		})
	}
}
