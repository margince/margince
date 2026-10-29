// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/pkg/extension"
)

// A refusing write hook rolls back the whole write through the real store:
// the deal, its audit row and its outbox event. An ordinary create runs the
// hook inside the same transaction and commits.
func TestAWriteHookRefusalRollsBackTheRealWrite(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	const refused = "write hook refuses this deal"
	var seen []string
	storekit.SetWriteHook(dispatchWriteHooks)
	t.Cleanup(func() { storekit.SetWriteHook(nil) })
	withComposed(t, extension.Extension{Name: "acme", WriteHooks: []extension.WriteHook{{
		Entity: "deal", Actions: []string{"create"},
		Handle: func(_ context.Context, _ extension.Runtime, w extension.Write) error {
			seen = append(seen, w.ID)
			if bytes.Contains(w.After, []byte(refused)) {
				return extension.ErrConflict
			}
			return nil
		},
	}}})
	store := deals.NewStore(e.DB(), DealsInstallation())
	pipeline, stage, _ := integration.DealFixture(t, e)

	kept, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: "Kept", PipelineID: pipeline, StageID: stage, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != kept.Id.String() {
		t.Fatalf("hook saw %v, want the kept deal's id", seen)
	}
	if _, err := store.CreateDeal(ctx, deals.CreateDealInput{Name: refused, PipelineID: pipeline, StageID: stage, Source: "manual"}); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("err = %v, want the hook's refusal as a conflict", err)
	}
	err = e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var dealRows, auditRows, outboxRows int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM deal WHERE name = $1`, refused).Scan(&dealRows); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'deal' AND after->>'name' = $1`, refused).Scan(&auditRows); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM event_outbox WHERE envelope::text LIKE '%' || $1 || '%'`, refused).Scan(&outboxRows); err != nil {
			return err
		}
		if dealRows+auditRows+outboxRows != 0 {
			t.Errorf("refused write left %d deal, %d audit and %d outbox rows, want none", dealRows, auditRows, outboxRows)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
