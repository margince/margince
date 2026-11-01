// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// One switch for the whole read: every statement of one Worklist page reuses
// its generic plan. The unchanged engine answers both ways; the answers must
// be the same and the time is compared.
func TestWorklistWholeReadPlanReusePoC(t *testing.T) {
	owner, app := pocPools(t)
	asOf := time.Now().UTC()
	service := newAttentionService(app, NewApprovalsService(InstallationDB(app)), func() time.Time { return asOf })
	read := func(ctx context.Context, generic bool, scope string) (crmcontracts.Worklist, time.Duration, error) {
		ctx = principal.WithCorrelationID(ctx, ids.UUID(uuid.Must(uuid.NewV7())))
		var out crmcontracts.Worklist
		start := time.Now()
		err := database.WithWorkspaceSnapshot(ctx, app, func(ctx context.Context) error {
			page := func(ctx context.Context) error {
				var err error
				out, err = service.WorklistView(ctx, "today", scope, "", ids.Nil, 25, "")
				return err
			}
			if generic {
				return database.ReuseGenericPlans(ctx, page)
			}
			return page(ctx)
		})
		return out, time.Since(start), err
	}
	fingerprint := func(w crmcontracts.Worklist) string {
		rows := make([]string, 0, len(w.Queue))
		for _, row := range w.Queue {
			rows = append(rows, string(row.Source)+":"+row.Id)
		}
		counts, _ := json.Marshal(w.Counts)
		summary, _ := json.Marshal(w.Summary)
		readings, _ := json.Marshal(w.Readings)
		return fmt.Sprint(rows, string(counts), string(summary), string(readings), len(w.SourcesUnavailable))
	}
	for _, scope := range []string{"mine", "team"} {
		var defTotal, genTotal time.Duration
		same, n := 0, 0
		for _, m := range pocMembers(t, owner, app) {
			var def, gen crmcontracts.Worklist
			var defTook, genTook []time.Duration
			failed := false
			for i := range 4 { // the first of each warms the connection's statement cache
				d, took, err := read(m.ctx, false, scope)
				if err != nil {
					failed = true
					break
				}
				g, gtook, err := read(m.ctx, true, scope)
				if err != nil {
					t.Fatalf("%s %s generic: %v", m.id, scope, err)
				}
				if i > 0 {
					defTook, genTook = append(defTook, took), append(genTook, gtook)
				}
				def, gen = d, g
			}
			if failed {
				continue // this member's role does not reach the scope
			}
			slices.Sort(defTook)
			slices.Sort(genTook)
			n++
			defTotal += defTook[1]
			genTotal += genTook[1]
			if fingerprint(def) == fingerprint(gen) {
				same++
			} else {
				t.Errorf("%s %s: the answer differs with generic plans", m.id, scope)
			}
			t.Logf("%-5s %s default %v, generic %v", scope, m.id.String()[24:], defTook[1].Round(time.Millisecond), genTook[1].Round(time.Millisecond))
		}
		if n > 0 {
			t.Logf("%s: %d members, identical answers for %d; mean default %v, generic %v", scope, n, same,
				(defTotal / time.Duration(n)).Round(time.Millisecond), (genTotal / time.Duration(n)).Round(time.Millisecond))
		}
	}
}
