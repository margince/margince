// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedLifecycleRepairWithdrawsContradictedChangesAndPreservesUncertainty(t *testing.T) {
	for _, change := range []string{"contract", "project", "workstream", "unknown"} {
		t.Run(change, func(t *testing.T) {
			e := integration.Setup(t)
			company := seedAccountAtStage(t, e, "customer")
			seedOpenContractEnded(t, e, company)
			if proposeAutomaticRepairLifecycle(t, e) != 1 {
				t.Fatal("fixture did not stage the account ending")
			}
			if change == "contract" || change == "project" {
				if err := prepareContinuityChange(t, e, company, change)(e.Admin()); err != nil {
					t.Fatal(err)
				}
			} else {
				at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
				seedScopedContractEndedAt(t, e, company, change, "The pilot is ending; the account scope needs review.", at, at)
			}
			operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
			now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
			if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
				review, err := previewHomeRepairSourceTx(operator, tx, "approval", ids.Nil, 10, now)
				if err != nil {
					return err
				}
				var scope homeRepairScope
				if err := json.Unmarshal(review.Scope, &scope); err != nil {
					return err
				}
				want := repairWithdrawProposal
				if change == "unknown" {
					want = repairHumanReview
				}
				if len(scope.Tasks) != 1 || scope.Tasks[0].Action != want {
					t.Fatalf("review=%+v, want %s", scope, want)
				}
				changed, err := applyHomeRepairProposal(operator, tx, scope.Tasks[0], datatask.Activation{ReviewRequest: review}, ids.NewV7())
				if err != nil {
					return err
				}
				if changed != (want == repairWithdrawProposal) {
					t.Fatalf("changed=%v, disposition=%s", changed, want)
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if proposeAutomaticRepairLifecycle(t, e) != 0 || accountStage(t, e, company) != "customer" {
				t.Fatal("repair recreated a contradicted account change or altered the account")
			}
			want := 1
			if change != "unknown" {
				want = 0
			}
			if got := e.WsCount(t, "SELECT count(*) FROM approval WHERE status='pending'"); got != want {
				t.Fatalf("pending=%d, want %d", got, want)
			}
		})
	}
}

func proposeAutomaticRepairLifecycle(t *testing.T, e *integration.Env) int {
	t.Helper()
	proposer := NewSignalProposer(e.Pool, approvalsServiceWithEffects(e.Pool), slog.New(slog.DiscardHandler))
	ctx := principal.SystemActing(e.As(e.AdminUser, nil, integration.AdminWithSignals), "system:signal-propose")
	count, err := proposer.RunWorkspace(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestReviewedLifecycleRepairPreservesAProposalThatBecomesCurrentAgain(t *testing.T) {
	e := integration.Setup(t)
	company := seedAccountAtStage(t, e, "customer")
	seedOpenContractEnded(t, e, company)
	if proposeAutomaticRepairLifecycle(t, e) != 1 {
		t.Fatal("fixture did not stage the account ending")
	}
	if err := prepareContinuityChange(t, e, company, "contract")(e.Admin()); err != nil {
		t.Fatal(err)
	}
	operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	var review datatask.ReviewRequest
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		var err error
		review, err = previewHomeRepairSourceTx(operator, tx, "approval", ids.Nil, 10, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var scope homeRepairScope
	if err := json.Unmarshal(review.Scope, &scope); err != nil {
		t.Fatal(err)
	}
	if len(scope.Tasks) != 1 || scope.Tasks[0].Action != repairWithdrawProposal {
		t.Fatalf("review=%+v", scope)
	}
	agreement, err := ids.Parse(e.WsScalar(t, "SELECT id::text FROM contract WHERE company_id=$1", company))
	if err != nil {
		t.Fatal(err)
	}
	ended := now.AddDate(0, 0, -1)
	if _, err := e.Contracts.Cancel(e.Admin(), ids.From[ids.ContractKind](agreement), ended, ended, nil); err != nil {
		t.Fatal(err)
	}
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		changed, err := applyHomeRepairProposal(operator, tx, scope.Tasks[0], datatask.Activation{ReviewRequest: review}, ids.NewV7())
		if err != nil {
			return err
		}
		if changed {
			t.Fatal("repair withdrew a proposal whose contradiction no longer holds")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, "SELECT count(*) FROM approval WHERE status='pending'") != 1 || accountStage(t, e, company) != "customer" {
		t.Fatal("repair lost the current proposal or changed the account")
	}
}
