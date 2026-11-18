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

// planNode is the part of an EXPLAIN node the test reads. Actual Rows is per
// loop, and a parallel node reports the sum of its workers' loops.
type planNode struct {
	Relation string     `json:"Relation Name"`
	Rows     float64    `json:"Actual Rows"`
	Loops    float64    `json:"Actual Loops"`
	Plans    []planNode `json:"Plans"`
}

// rowsRead sums the rows every node on these tables produced, workers included.
func (n planNode) rowsRead(tables map[string]bool) (rows float64, nodes int) {
	if tables[n.Relation] {
		rows, nodes = n.Rows*n.Loops, 1
	}
	for _, child := range n.Plans {
		childRows, childNodes := child.rowsRead(tables)
		rows, nodes = rows+childRows, nodes+childNodes
	}
	return rows, nodes
}

// A colleague at the candidate's employer is not a candidate by that alone. If
// it were, an import filing contacts at one company would read every earlier
// colleague for each new row.
//
// Mutation: restore the relationship branch of contactCandidateQuery, which
// unions in every current colleague of @employer.
func TestTheCandidateReadSkipsColleaguesWhoseNamesDoNotMatch(t *testing.T) {
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
	rows, nodes := plans[0].Plan.rowsRead(map[string]bool{"contact": true, "relationship": true})
	if nodes == 0 {
		t.Fatalf("the plan reads neither contact nor relationship, so it measures nothing: %s", raw)
	}
	if rows > dedupeCandidateRows {
		t.Fatalf("the candidate read took %.0f rows from contact and relationship for %d unrelated colleagues, want at most %d",
			rows, staff, dedupeCandidateRows)
	}
}
