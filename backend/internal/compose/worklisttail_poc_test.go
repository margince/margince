// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/margince/margince/backend/internal/compose/attention"
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
	stages := []string{"rank", "tail", "tail_task", "tail_customer_waiting", "tail_deal_at_risk"}
	total := map[string][2]float64{}
	pages := 0
	for i, m := range pocMembers(t, owner, app) {
		for _, scope := range []string{"mine", "team"} {
			token := ""
			for page := 1; page <= 2; page++ {
				before, beforeTime := attention.StageCosts(), attention.StageTimings()
				ctx := principal.WithCorrelationID(m.ctx, ids.UUID(uuid.Must(uuid.NewV7())))
				start := time.Now()
				list, err := service.WorklistView(ctx, "today", scope, "", ids.Nil, 100, token)
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
				pages++
				t.Log(line)
				if list.NextCursor == nil || *list.NextCursor == "" {
					break
				}
				token = *list.NextCursor
			}
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
