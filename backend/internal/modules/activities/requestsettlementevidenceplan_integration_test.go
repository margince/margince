// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The candidate read evaluates this evidence once per request already judged,
// so a whole-table read here is paid per verdict ever written.
// With sequential scans priced out, a predicate no index serves becomes a full
// primary-key walk: an index scan with no index condition.
func TestSettlementEvidenceReadsEachActivityByItsID(t *testing.T) {
	plan := explainWithoutSeqScans(t, "",
		`SELECT (`+settlementEvidenceSQL()+`) FROM activity a WHERE a.id = $2`,
		clocktest.Now(t), ids.NewV7())
	var seenReads, unkeyed int
	plan.walk(func(n planNode) {
		if planCopySuffix.ReplaceAllString(n.Alias, "") != "seen" {
			return
		}
		seenReads++
		if n.IndexCond == "" {
			unkeyed++
		}
	})
	if seenReads == 0 {
		t.Fatal("the plan reads no activity under the alias seen: the evidence was not rendered, so this test checks nothing")
	}
	if unkeyed > 0 {
		t.Errorf("the evidence reads the whole activity table (%d reads of seen with no index condition): the request and its answers are not looked up by id", unkeyed)
	}
}
