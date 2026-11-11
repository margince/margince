// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

func TestEveryCapturePurgeKeepsAMessagePinnedAfterSelection(t *testing.T) {
	for _, withCheck := range []bool{false, true} {
		t.Run(map[bool]string{false: "owner rule", true: "mailbox deletion"}[withCheck], func(t *testing.T) {
			e := Setup(t)
			mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "email", Subject: new("Private note"), Body: new("The original message."), Source: "manual", Direction: new("inbound"),
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.UUID(mail.Id)
			reason, err := privacy.ParseStatedReason("Keep the correspondence for the legal record.")
			if err != nil {
				t.Fatal(err)
			}
			controller := controllerCtx(e, principal.ObjectGrant{Read: true, Update: true})
			if err := privacy.NewEraser(e.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).PinToFloor(controller, id, reason); err != nil {
				t.Fatal(err)
			}
			// The restriction must hold even when the current jurisdiction has no floor.
			retention := compose.NewRetentionServiceFor(e.DB(), nil, slog.Default()).WithCountry(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return "zk", nil })
			var check privacy.ActivityStillPurgeable
			if withCheck {
				check = func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return true, nil }
			}
			count, err := retention.PurgeActivitiesIf(e.Admin(), []ids.UUID{id}, privacy.PurgeOwnerRule, check)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 || e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND body=$2`, id, "The original message.") != 1 {
				t.Fatal("purge destroyed a pinned message")
			}
		})
	}
}
