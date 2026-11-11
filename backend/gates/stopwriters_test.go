// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httpserver"
)

// Every mutation coordinates: restoring a stop or widening its purpose can
// revoke permission without changing either address column. Deletion cannot.
func TestEveryStopMutationOrdersWithProviderHandoff(t *testing.T) {
	t.Parallel()
	expected := map[string]map[string]bool{}
	for _, writes := range collectTableWrites(t) {
		for _, write := range writes {
			if write.table != "communication_suppression" || write.verb == "delete" {
				continue
			}
			_, name, ok := strings.Cut(write.site, ":")
			location := strings.Split(write.pos, ":")
			if !ok || len(location) < 3 {
				t.Fatalf("stop writer has no function/position: %+v", write)
			}
			dir := filepath.ToSlash(filepath.Dir(strings.Join(location[:len(location)-2], ":")))
			if expected[dir] == nil {
				expected[dir] = map[string]bool{}
			}
			expected[dir][name] = true
		}
	}
	checked := 0
	for dir, flow := range linePackageFlows(t) {
		observed, problems := stopWriterProblems(flow, expected[dir])
		for _, problem := range problems {
			t.Errorf("%s: %s", dir, problem)
		}
		for name := range expected[dir] {
			if !observed[name] {
				t.Errorf("%s:%s is missing from the stop writer census", dir, name)
			}
			checked++
		}
		delete(expected, dir)
	}
	if len(expected) != 0 {
		t.Fatalf("stop writer packages disappeared: %v", expected)
	}
	if checked == 0 {
		t.Fatal("the stop writer census found no subjects")
	}
}

func stopWriterProblems(flow *lineLockFlow, expected map[string]bool) (map[string]bool, []string) {
	observed := map[string]bool{}
	var problems []string
	check := func(name string, pos token.Pos) {
		observed[name] = true
		if name == "" || !flow.handledBefore(name, pos, func(call *ast.CallExpr) bool {
			const path = "github.com/margince/margince/backend/internal/platform/database/storekit"
			return unshadowedPackageCall(flow.files[name], flow.locals[name], call.Fun, path, "LockStopAddresses") || unshadowedPackageCall(flow.files[name], flow.locals[name], call.Fun, path, "LockSubjectsStopAddresses")
		}) {
			problems = append(problems, fmt.Sprintf("%s changes an address-bound stop before taking its shared address keys", name))
		}
	}
	for _, file := range flow.corpus {
		for _, statement := range sqlStrings(file, flow.constants) {
			for _, write := range sqlWrites(statement.text) {
				if write.table != "communication_suppression" || write.verb == "delete" {
					continue
				}
				name := ""
				for candidate, fn := range flow.functions {
					if fn.Pos() <= statement.pos && statement.pos < fn.End() {
						name = candidate
					}
				}
				check(name, statement.pos)
			}
		}
	}
	for _, site := range flow.dealPatchSites("communication_suppression") {
		if site.tables["communication_suppression"] || expected[site.name] {
			check(site.name, site.call.Pos())
		}
	}
	return observed, problems
}

func TestTheProviderHandoffBudgetLeavesTimeToRecordAStop(t *testing.T) {
	t.Parallel()
	if comms.ProviderHandoffBudget <= 0 || comms.ProviderHandoffBudget+database.SessionLeaseCleanupTimeout >= httpserver.ResponseDeadline || comms.ProviderHandoffBudget+database.SessionLeaseCleanupTimeout >= database.StatementCeiling {
		t.Fatalf("provider handoff plus cleanup %s must end before response %s and statement %s budgets", comms.ProviderHandoffBudget+database.SessionLeaseCleanupTimeout, httpserver.ResponseDeadline, database.StatementCeiling)
	}
}

func TestTheStopWriterCensusRejectsPlantedBypasses(t *testing.T) {
	t.Parallel()
	const write = `tx.Exec(ctx,"INSERT INTO communication_suppression(address) VALUES (@address)",args); return nil`
	const guard = `if err:=sk.LockStopAddresses(ctx,tx,addresses,nil);err!=nil{return err};`
	for _, tc := range []struct {
		name, body string
		allowed    bool
	}{
		{"guarded", guard + write, true},
		{"bare", write, false},
		{"scope change", `tx.Exec(ctx,"UPDATE communication_suppression SET purpose_id=NULL",args);return nil`, false},
		{"restore", `tx.Exec(ctx,"UPDATE communication_suppression SET revoked_at=NULL",args);return nil`, false},
		{"quoted hash", `tx.Exec(ctx,"INSERT INTO \"public\".\"communication_suppression\"(\"address_hash\") VALUES (@hash)");return nil`, false},
		{"late", `tx.Exec(ctx,"INSERT INTO communication_suppression(address) VALUES (@address)",args);` + guard + `return nil`, false},
		{"conditional", `if chosen {` + guard + `};` + write, false},
		{"unused closure", `unused:=func()error{` + guard + `return nil};` + write, false},
		{"shadowed package", `sk:=other;` + guard + write, false},
		{"unhandled error", `sk.LockStopAddresses(ctx,tx,addresses,nil);` + write, false},
		{"generic patch", `l,_:=sk.LockRow(ctx,tx,"communication_suppression",id,live);p.ApplyLocked(ctx,tx,l);return nil`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flow := lineFixture(t, `func save()error{`+tc.body+`}`)
			observed, problems := stopWriterProblems(flow, nil)
			if len(observed) == 0 {
				t.Fatal("the planted writer disappeared from the census")
			}
			if (len(problems) == 0) != tc.allowed {
				t.Fatalf("allowed=%v findings=%v", tc.allowed, problems)
			}
		})
	}
}
