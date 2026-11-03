// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A kind with a context pin refuses redemption and decision when the digest
// moved. A pending row staged before the pin is pinned when decided instead,
// while an approved row with no digest still refuses to redeem.
func TestAContextPinRefusesARowWhoseContextMovedOrWasNeverPinned(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mutate  func(e *stagingEnv, t *testing.T, digest *string, rows []ids.UUID)
		refused bool
		// decidable is whether the pending row may still be approved.
		decidable bool
	}{
		{"the context held", func(*stagingEnv, *testing.T, *string, []ids.UUID) {}, false, true},
		{"the context moved", func(_ *stagingEnv, _ *testing.T, digest *string, _ []ids.UUID) { *digest = "context-after" }, true, false},
		{"staged before the pin", func(e *stagingEnv, t *testing.T, _ *string, rows []ids.UUID) {
			if _, err := e.owner.Exec(context.Background(), `UPDATE approval SET context_digest = NULL WHERE id = ANY($1)`, rows); err != nil {
				t.Fatal(err)
			}
		}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupStaging(t)
			passport := e.seedPassport(t)
			current := "context-as-staged"
			e.svc.WithContextPin("merge_tags", func(context.Context, pgx.Tx, ids.UUID, json.RawMessage) (string, error) {
				return current, nil
			}).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
				return &crmcontracts.ApprovalPreview{Version: 2, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{}}, nil
			})
			approvedCall := e.mergeCall(e.seedTag(t, "PinSource"), e.seedTag(t, "PinTarget"))
			approved, err := e.svc.Stage(e.asPassport(passport), approvedCall)
			if err != nil {
				t.Fatal(err)
			}
			e.approve(t, approved)
			pendingCall := e.mergeCall(e.seedTag(t, "PinSource"), e.seedTag(t, "PinTarget"))
			pending, err := e.svc.Stage(e.asPassport(passport), pendingCall)
			if err != nil {
				t.Fatal(err)
			}
			tc.mutate(e, t, &current, []ids.UUID{approved.UUID, pending.UUID})

			_, _, err = e.svc.Redeem(e.asPassport(passport), approved, approvedCall.Kind, approvedCall.DiffHash)
			if errors.Is(err, apperrors.ErrVersionSkew) != tc.refused || (!tc.refused && err != nil) {
				t.Errorf("redeeming = %v, want refused=%t", err, tc.refused)
			}
			_, err = e.svc.Decide(e.asHumanWith(tagDeciderPerms()), pending, true, nil)
			if errors.Is(err, apperrors.ErrVersionSkew) == tc.decidable || (tc.decidable && err != nil) {
				t.Errorf("deciding = %v, want decidable=%t", err, tc.decidable)
			}
			if tc.decidable && e.count(t, `SELECT count(*) FROM approval WHERE id = $1 AND context_digest = $2`, pending.UUID, current) != 1 {
				t.Error("the decided row does not carry the context it was decided on")
			}
		})
	}
}

func tagDeciderPerms() principal.Permissions {
	return principal.Permissions{
		RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{
			"approval": {Create: true, Read: true, Update: true, Delete: true},
			"tag":      {Create: true, Read: true, Update: true, Delete: true},
		},
	}
}
