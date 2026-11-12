// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
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

func TestADeniedContextPinWithholdsOnePreviewWithoutBreakingTheInbox(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	denied := ids.Nil
	e.svc.WithContextPin("merge_tags", func(_ context.Context, _ pgx.Tx, target ids.UUID, _ json.RawMessage) (string, error) {
		if target == denied {
			return "", apperrors.ErrPermissionDenied
		}
		return "reviewed-context", nil
	}).WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		return &crmcontracts.ApprovalPreview{Version: 2, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{}}, nil
	})
	blocked := e.mergeCall(e.seedTag(t, "Blocked source"), e.seedTag(t, "Blocked target"))
	blockedID, err := e.svc.Stage(e.asPassport(passport), blocked)
	if err != nil {
		t.Fatal(err)
	}
	allowed := e.mergeCall(e.seedTag(t, "Allowed source"), e.seedTag(t, "Allowed target"))
	allowedID, err := e.svc.Stage(e.asPassport(passport), allowed)
	if err != nil {
		t.Fatal(err)
	}
	denied = blocked.TargetID
	human := e.asHumanWith(tagDeciderPerms())
	rows, _, err := e.svc.ListWire(human, ListInput{})
	if err != nil || len(rows) != 2 {
		t.Fatalf("inbox: rows=%d, error=%v", len(rows), err)
	}
	for _, row := range rows {
		want := "ready"
		if ids.UUID(row.Id) == blockedID.UUID {
			want = "stale"
		}
		if row.Preview == nil || row.Preview.State != want {
			t.Fatalf("approval %s preview=%+v, want %s", row.Id, row.Preview, want)
		}
	}
	if _, err := e.svc.Decide(human, blockedID, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("deciding the denied context: %v, want stale-context refusal", err)
	}
	if _, err := e.svc.Decide(human, allowedID, true, nil); err != nil {
		t.Fatalf("deciding the unaffected row: %v", err)
	}
}

func TestOnlyASingleApprovalReadBuildsItsPreviewAsOne(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	var reads []bool
	e.svc.WithPreview(func(ctx context.Context, _ crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		reads = append(reads, ReadsOneApproval(ctx))
		return &crmcontracts.ApprovalPreview{Version: 2, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{}}, nil
	})
	staged, err := e.svc.Stage(e.asPassport(passport), e.mergeCall(e.seedTag(t, "One source"), e.seedTag(t, "One target")))
	if err != nil {
		t.Fatal(err)
	}
	human := e.asHumanWith(tagDeciderPerms())
	if _, _, err := e.svc.ListWire(human, ListInput{}); err != nil {
		t.Fatal(err)
	}
	listed := slices.Clone(reads)
	if _, err := e.svc.GetWire(human, staged); err != nil {
		t.Fatal(err)
	}
	if len(listed) == 0 || slices.Contains(listed, true) || !reads[len(reads)-1] {
		t.Fatalf("list reads %v then single read %v, want false for the list and true for the single read", listed, reads[len(listed):])
	}
}
