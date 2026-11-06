// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDomainEvidenceSeeksPastAnEntireMatchingHistory(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	e.seedHistoryReadBackground(ctx, t, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	analyzeAsOwner(t, "activity", "activity_participant")
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		for _, domain := range []string{"unrelated.test", "mail.unrelated.test", "absent.test"} {
			t.Run(mode+"/"+domain, func(t *testing.T) {
				err := e.store.tx(ctx, func(nativeTx pgx.Tx) error {
					if _, err := nativeTx.Exec(ctx, "SET LOCAL plan_cache_mode="+mode); err != nil {
						return err
					}
					trace := &historyPlanTrace{needle: "SELECT NOT EXISTS("}
					tx := historyPlanTx{Tx: nativeTx, trace: trace}
					ready, err := storekit.ParticipantEvidenceReady(ctx, tx)
					if err != nil {
						return err
					}
					if !ready {
						return fmt.Errorf("complete fixture is not ready")
					}
					if err := assertDomainEvidencePlan(t, tx, trace, 64); err != nil {
						return err
					}
					trace.needle = "WITH projection AS MATERIALIZED"
					trace.query = ""
					at := newestEvidenceFor(ctx, tx, domain, ids.From[ids.ActivityKind](ids.Nil))
					if (at != nil) != (domain != "absent.test") {
						return fmt.Errorf("domain %s evidence=%v", domain, at)
					}
					if at != nil && !at.Equal(time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)) {
						return fmt.Errorf("unexpected evidence date %s", at)
					}
					return assertDomainEvidencePlan(t, tx, trace, 64)
				})
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func assertDomainEvidencePlan(t *testing.T, tx pgx.Tx, trace *historyPlanTrace, limit int) error {
	t.Helper()
	if trace.query == "" {
		return fmt.Errorf("production query was not recorded")
	}
	raw, err := testdb.ExplainCachedStatement(t.Context(), tx, trace.query, trace.args)
	if err != nil {
		return err
	}
	var plans []struct{ Plan mailboxPlanNode }
	if err := json.Unmarshal(raw, &plans); err != nil {
		return err
	}
	if len(plans) != 1 || !plans[0].Plan.boundsHistoryRows(limit) || plans[0].Plan.SharedHits+plans[0].Plan.SharedReads > 256 {
		return fmt.Errorf("domain evidence exceeds its fixed read bound: %s", raw)
	}
	t.Logf("bounded domain evidence: %s", raw)
	return nil
}
