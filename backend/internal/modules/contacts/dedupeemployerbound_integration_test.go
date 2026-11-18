// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
)

// planNode is the part of an EXPLAIN node the test reads.
type planNode struct {
	Type  string     `json:"Node Type"`
	Loops int        `json:"Actual Loops"`
	Plans []planNode `json:"Plans"`
}

// mostProbedSubquery is the highest loop count of any subquery scan: how many
// candidates the per-candidate point read of the contact row ran for.
func (n planNode) mostProbedSubquery() int {
	most := 0
	if n.Type == "Subquery Scan" {
		most = n.Loops
	}
	for _, child := range n.Plans {
		most = max(most, child.mostProbedSubquery())
	}
	return most
}

// The candidate read does one point read per candidate. An employer's whole
// staff must not become candidates, or an import filing contacts at one
// company reads every earlier colleague for each new row.
//
// Mutation: drop the LIMIT in the same_employer branch of contactCandidateQuery.
func TestTheCandidateReadStaysBoundedWhenManyContactsShareAnEmployer(t *testing.T) {
	e := setupDedupe(t)
	employer := mustEmployer(t, e, "Crowded Works")
	const staff = 3 * dedupeCandidateRows
	for i := range staff {
		employ(t, e, mustContact(t, e, fmt.Sprintf("Zq%dxv Wkrmpl%d", i*7919, i)), employer)
	}
	var raw []byte
	err := e.store.tx(e.as(), func(tx pgx.Tx) error {
		args := contactCandidateArgs(ContactCandidate{FullName: "Hjalmar Quenby", CurrentPrimaryCompanyID: &employer})
		return tx.QueryRow(e.as(), "EXPLAIN (ANALYZE, FORMAT JSON) "+contactCandidateQuery, args).Scan(&raw)
	})
	if err != nil {
		t.Fatalf("explaining the candidate read: %v", err)
	}
	var plans []struct {
		Plan planNode `json:"Plan"`
	}
	if err := json.Unmarshal(raw, &plans); err != nil || len(plans) != 1 {
		t.Fatalf("reading the plan: %v", err)
	}
	if got := plans[0].Plan.mostProbedSubquery(); got > dedupeCandidateRows {
		t.Fatalf("the candidate read probed %d candidates for %d colleagues, want at most %d", got, staff, dedupeCandidateRows)
	}
}
