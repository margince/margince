// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httpserver

import (
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TestEveryPoolStatisticIsExported asks the POOL what it computes rather than
// keeping a list of what it ought to.
//
// This is the defect's own shape. The exposition published four of the values
// pgxpool.Stat answers and dropped the rest, and nothing failed: a metric that
// is absent looks exactly like a metric that is zero, and the four that
// remained were the four that could not show a queue. A list of expected names
// here would be a second copy of the exporter and would have been written from
// the same four.
//
// A value pgx adds in a later release arrives as an unexported statistic and
// fails on the upgrade's own pull request, which is the moment somebody is
// already reading this package.
func TestEveryPoolStatisticIsExported(t *testing.T) {
	t.Parallel()
	exported := map[string]string{}
	for _, level := range poolLevels {
		exported[level.stat] = "margince_pgxpool_conns{state=" + level.state + "}"
	}
	for _, counter := range poolCounters {
		if was, dup := exported[counter.stat]; dup {
			t.Errorf("%s is rendered twice, as %s and as %s", counter.stat, was, counter.name)
		}
		exported[counter.stat] = counter.name
	}

	stat := reflect.TypeOf(&pgxpool.Stat{})
	var missing []string
	for i := range stat.NumMethod() {
		name := stat.Method(i).Name
		if _, ok := exported[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		t.Errorf("pgxpool.Stat computes %d statistic(s) this process never publishes:\n\t%s\n"+
			"Each is free — the value is already on the stat the exposition holds — and an operator cannot ask for one that was never emitted. Render it beside the others, or say here why the pool's own measurement is not worth a line.",
			len(missing), strings.Join(missing, "\n\t"))
	}
	// The reverse: a name that no longer answers anything renders a metric
	// nobody can explain, and the compiler cannot see it because the table
	// carries the method as a string beside the call.
	for name := range exported {
		if _, ok := stat.MethodByName(name); !ok {
			t.Errorf("the exposition names pgxpool.Stat.%s, which the pool no longer computes", name)
		}
	}
	if stat.NumMethod() == 0 {
		t.Fatal("read no statistic off pgxpool.Stat at all — this type has always had some, so the census read something smaller than it claims")
	}
}

// TestThePoolsWaitsAreCountersAndItsLevelsAreGauges pins the split the board
// depends on. A level answers only for the instant it was scraped, so at a
// five-minute interval a queue that formed and drained between two scrapes
// leaves no trace; a counter carries it into the next one regardless. Emitting
// a wait as a gauge would publish the series and lose exactly the episodes it
// was added for.
func TestThePoolsWaitsAreCountersAndItsLevelsAreGauges(t *testing.T) {
	t.Parallel()
	// pgx names a running total Count, and an accumulated one Time or
	// Duration; everything else it answers is a reading of the pool now.
	for _, level := range poolLevels {
		for _, monotonic := range []string{"Count", "Time", "Duration"} {
			if strings.HasSuffix(level.stat, monotonic) {
				t.Errorf("%s is rendered as a gauge; a statistic that only goes up is a counter, and a scrape that misses its episode must still see it", level.stat)
			}
		}
	}
	for _, counter := range poolCounters {
		if strings.HasSuffix(counter.stat, "Conns") {
			t.Errorf("%s is rendered as a counter; it is a reading of the pool now, and a counter that goes down is one Prometheus reads as a reset", counter.stat)
		}
	}
	for _, counter := range poolCounters {
		if !strings.HasSuffix(counter.name, "_total") {
			t.Errorf("%s is a counter named without the _total suffix Prometheus reads it by", counter.name)
		}
		if strings.Contains(counter.name, "seconds") && !strings.HasSuffix(counter.name, "_seconds_total") {
			t.Errorf("%s reports seconds without saying so where the unit is read", counter.name)
		}
	}
}

// TestTheOperatorDocNamesEveryPoolSeries holds the reference to the exporter.
//
// The doc is where an operator finds out a series exists, so a family that is
// emitted and undocumented is one nobody queries, and a family documented and
// never emitted is a query that returns nothing with no error to explain it.
// Both are silent, which is why the parity is a test rather than a habit.
func TestTheOperatorDocNamesEveryPoolSeries(t *testing.T) {
	t.Parallel()
	const doc = "../../../../docs/reference/configuration.md"
	raw, err := os.ReadFile(doc)
	if err != nil {
		t.Fatalf("reading the operator reference: %v", err)
	}
	text := string(raw)

	for _, name := range append(poolCounterNames(), "margince_pgxpool_conns") {
		if !strings.Contains(text, name) {
			t.Errorf("%s is published and %s never names it, so an operator has no way to learn it exists", name, doc)
		}
	}
	for _, level := range poolLevels {
		if !strings.Contains(text, "`"+level.state+"`") {
			t.Errorf("the pool publishes state=%q and %s does not list it among the states", level.state, doc)
		}
	}
	// And the other direction, which is the half a hand-written list gets
	// wrong: a name the doc invented, or one left behind by a rename.
	for _, documented := range regexp.MustCompile(`margince_pgxpool[a-z_]*`).FindAllString(text, -1) {
		if documented == "margince_pgxpool_conns" || documented == "margince_pgxpool_" {
			continue
		}
		if !slices.Contains(poolCounterNames(), documented) {
			t.Errorf("%s documents %s, which nothing publishes", doc, documented)
		}
	}
}

func poolCounterNames() []string {
	names := make([]string, 0, len(poolCounters))
	for _, counter := range poolCounters {
		names = append(names, counter.name)
	}
	return names
}
