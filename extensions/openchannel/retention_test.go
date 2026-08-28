// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Nothing outside this unit will ever clean these tables: core retention walks
// tables core knows about, and a unit's are not among them. Both of them are fed
// by parties with no session, so a connector that did not sweep would let a
// remote party decide how much this installation stores, forever.
func TestTheDrainSweepsBothOfItsGrowingTables(t *testing.T) {
	t.Parallel()
	rt := draining()
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("draining: %v", err)
	}
	// Statements are found by the retention window they bind rather than by a
	// needle shaped like the start of one: the tree's SQL-scope gate reads string
	// literals looking for the table a statement names, and a literal that opens
	// a statement reads as SQL whose table it cannot resolve.
	for _, table := range []string{inboundTable, outboundTable} {
		if !sweeps(rt, table) {
			t.Fatalf("%s is never swept, so it grows for the life of the installation", table)
		}
	}
}

// A PENDING request is never swept at any age. It is a message somebody was told
// had been accepted, and the drain's own parking is what turns one nothing will
// land into a decided row — so an aged-out queue cannot silently swallow work.
func TestTheSweepNeverRemovesARequestNobodyHasDecidedAbout(t *testing.T) {
	t.Parallel()
	rt := draining()
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("draining: %v", err)
	}
	sql, args := rt.tx.statementMentioning(t, "state <> $1")
	if args[0] != stateWaiting {
		t.Fatalf("the sweep spares state %v", args[0])
	}
	if args[1] != retainDecidedDays {
		t.Fatalf("the sweep keeps decided requests for %v days, and the window is %d", args[1], retainDecidedDays)
	}
	if !strings.Contains(sql, "received_at < now()") {
		t.Fatalf("the sweep is not bounded by arrival:\n%s", sql)
	}
}

// The two tables answer mirror questions — what arrived, and what left — so a
// member comparing the two screens must not find one of them remembering a month
// further back than the other.
func TestBothLedgersRememberForTheSameLength(t *testing.T) {
	t.Parallel()
	rt := draining()
	if err := drain(context.Background(), rt); err != nil {
		t.Fatalf("draining: %v", err)
	}
	windows := map[any]bool{}
	for at, sql := range rt.tx.statements {
		if strings.Contains(sql, retentionNeedle) {
			windows[rt.tx.args[at][len(rt.tx.args[at])-1]] = true
		}
	}
	if len(windows) != 1 {
		t.Fatalf("the two ledgers are swept on %d different windows: %v", len(windows), windows)
	}
}

// retentionNeedle is how a sweep is recognised in the statements a tick issued.
const retentionNeedle = "make_interval(days"

// sweeps reports whether the tick asked for one table's aged rows to go.
func sweeps(rt *fakeRuntime, table string) bool {
	for _, sql := range rt.tx.statements {
		if strings.Contains(sql, retentionNeedle) && strings.Contains(sql, table) {
			return true
		}
	}
	return false
}

// The cadence this connector ticks at and the delay it postpones by are ONE
// number in two files, which the tier's own parity gate reconciles. This holds
// the same pair from inside the unit, so a fragment edited here fails here rather
// than in a gate somebody runs later.
func TestThePostponementIsTheCadenceTheDispatcherAlreadyTicksAt(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("api/jobs.yaml")
	if err != nil {
		t.Fatalf("reading the jobs fragment: %v", err)
	}
	found := regexp.MustCompile(`(?m)^\s*cadence:\s*(\S+)\s*$`).FindAllStringSubmatch(string(raw), -1)
	if len(found) != 1 {
		t.Fatalf("the fragment declares %d cadences; which one the postponement agrees with would be decided by file order", len(found))
	}
	// Parsed rather than compared as text: the two files spell a duration in two
	// grammars, and normalising them is the whole point of comparing them.
	cadence, err := time.ParseDuration(found[0][1])
	if err != nil {
		t.Fatalf("the fragment declares cadence %q, which is not a duration: %v", found[0][1], err)
	}
	if cadence != pollRetryDelay {
		t.Fatalf("the dispatcher ticks every %s and a stalled tick postpones by %s — a shorter delay drains a broken installation harder during an outage than in health, and a longer one lets the dispatcher insert a second tick before the postponed row wakes",
			cadence, pollRetryDelay)
	}
}
