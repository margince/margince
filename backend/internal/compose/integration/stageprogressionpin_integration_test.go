// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestProgressionCannotRedeemAfterItsDestinationChanges(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("the real proposer staged no move")
	}
	var approvalID ids.ApprovalID
	var target ids.StageID
	var version int64
	var hash string
	if err := e.Pool.QueryRow(t.Context(), `SELECT id, co_target_entity_id, co_target_version, diff_hash
 FROM approval WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending' AND co_target_entity_type = 'stage'`,
		deals.StageProgressionKind, deal).Scan(&approvalID, &target, &version, &hash); err != nil {
		t.Fatal(err)
	}
	stage, err := e.Deals.GetStage(e.Admin(), target)
	if err != nil {
		t.Fatal(err)
	}
	if stage.Version == nil || int64(*stage.Version) != version {
		t.Fatalf("pin=%d, stage=%v", version, stage.Version)
	}
	svc := approvals.NewService(e.DB())
	if _, err := svc.Decide(e.Admin(), approvalID, true, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.UpdateStage(e.Admin(), target, deals.UpdateStageInput{Name: new("Revised destination"), IfVersion: &version}); err != nil {
		t.Fatal(err)
	}
	applied := false
	err = svc.RedeemAndApply(e.Admin(), approvalID, deals.StageProgressionKind, hash, func(pgx.Tx) error {
		applied = true
		return nil
	})
	if !errors.Is(err, apperrors.ErrVersionSkew) || applied {
		t.Fatalf("changed destination: applied=%t err=%v", applied, err)
	}
	var consumed bool
	if err := e.Pool.QueryRow(t.Context(), `SELECT consumed_at IS NOT NULL FROM approval WHERE id = $1`, approvalID).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	if consumed {
		t.Fatal("refused move consumed its approval")
	}
}
