// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// What the scroll tail costs on each member's first screen page and on the
// page after it: statements and time per stage, from the stage metrics.
func TestWorklistTailCostPoC(t *testing.T) {
	owner, app := pocPools(t)
	asOf := time.Now().UTC()
	if pinned := os.Getenv("POC_AS_OF"); pinned != "" {
		var err error
		if asOf, err = time.Parse(time.RFC3339, pinned); err != nil {
			t.Fatal(err)
		}
	}
	service := newAttentionService(app, NewApprovalsService(InstallationDB(app)), func() time.Time { return asOf })
	stages := []string{"waiting_customers", "rank", "tail", "tail_task", "tail_customer_waiting", "tail_deal_at_risk"}
	total := map[string][2]float64{}
	every, everyStatements := map[string]float64{}, map[string]int64{}
	var wholes []time.Duration
	pages := 0
	members := pocMembers(t, owner, app)
	passes, _ := strconv.Atoi(os.Getenv("POC_PASSES"))
	for pass := 1; pass < passes; pass++ {
		for _, m := range members {
			for _, scope := range []string{"mine", "team"} {
				if _, err := service.WorklistView(m.ctx, "today", scope, "", ids.Nil, 100, ""); err != nil {
					continue // a scope this member may not read
				}
			}
		}
	}
	for i, m := range members {
		for _, scope := range []string{"mine", "team"} {
			token := ""
			for page := 1; page <= 2; page++ {
				before, beforeTime := attention.StageCosts(), attention.StageTimings()
				ctx := principal.WithCorrelationID(m.ctx, ids.UUID(uuid.Must(uuid.NewV7())))
				start := time.Now()
				var list crmcontracts.Worklist
				err := database.WithWorkspaceSnapshot(ctx, app, func(ctx context.Context) error {
					page := func(ctx context.Context) error {
						var err error
						list, err = service.WorklistView(ctx, "today", scope, "", ids.Nil, 100, token)
						return err
					}
					if os.Getenv("POC_PLANS") == "generic" {
						return database.ReuseGenericPlans(ctx, page)
					}
					return page(ctx)
				})
				if err != nil {
					break // a scope this member may not read
				}
				whole := time.Since(start)
				after, afterTime := attention.StageCosts(), attention.StageTimings()
				line := fmt.Sprintf("member=%d scope=%s page=%d rows=%d whole=%dms", i, scope, page, len(list.Queue), whole.Milliseconds())
				for _, stage := range stages {
					key := attention.StageKey{Read: attention.ReadWorklist, Stage: stage}
					statements := after[key].Statements - before[key].Statements
					spent := afterTime[key].Sum() - beforeTime[key].Sum()
					line += fmt.Sprintf(" %s=%d/%.0fms", stage, statements, spent*1000)
					sum := total[stage]
					total[stage] = [2]float64{sum[0] + float64(statements), sum[1] + spent*1000}
				}
				for key := range after {
					if key.Read == attention.ReadWorklist {
						every[key.Stage] += afterTime[key].Sum() - beforeTime[key].Sum()
						everyStatements[key.Stage] += after[key].Statements - before[key].Statements
					}
				}
				wholes = append(wholes, whole)
				if os.Getenv("POC_ALL_STAGES") == strconv.Itoa(i) {
					for key, cost := range after {
						spent := afterTime[key].Sum() - beforeTime[key].Sum()
						if key.Read == attention.ReadWorklist && spent > 0.02 {
							t.Logf("  stage %-24s %5d statements %6.0f ms", key.Stage, cost.Statements-before[key].Statements, spent*1000)
						}
					}
				}
				pages++
				t.Log(line)
				if list.NextCursor == nil || *list.NextCursor == "" {
					break
				}
				token = *list.NextCursor
			}
		}
	}
	sort.Slice(wholes, func(a, b int) bool { return wholes[a] < wholes[b] })
	if len(wholes) > 0 {
		t.Logf("whole read over %d pages: p50 %dms p90 %dms max %dms", len(wholes),
			wholes[len(wholes)/2].Milliseconds(), wholes[len(wholes)*9/10].Milliseconds(), wholes[len(wholes)-1].Milliseconds())
	}
	for stage, spent := range every {
		if spent/float64(pages) >= 0.02 {
			t.Logf("stage mean %-24s %6.0f ms %6.1f statements", stage, spent*1000/float64(pages), float64(everyStatements[stage])/float64(pages))
		}
	}
	names := make([]string, 0, len(total))
	for stage := range total {
		names = append(names, stage)
	}
	sort.Strings(names)
	for _, stage := range names {
		t.Logf("mean over %d pages: %s statements=%.1f time=%.0fms", pages, stage, total[stage][0]/float64(pages), total[stage][1]/float64(pages))
	}
}
