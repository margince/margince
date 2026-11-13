// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const vitestLane lane = "vitest"

// budget is what one lane may spend. A cheap selection runs `most` times, and a
// costly one fewer times, but never fewer than `fewest`. Past that the lane
// leaves tests out. It drops the tests the diff reaches only through a helper
// first.
type budget struct {
	most, fewest int
	spend        float64
}

// The budgets, measured on recent pull requests (see the PR that added this
// tool). A unit test costs 1, an integration test its seconds in the lane's
// timing file, a vitest file 1 for each attempt.
var budgets = map[lane]budget{
	unitLane:        {most: 20, fewest: 3, spend: 1000},
	integrationLane: {most: 10, fewest: 3, spend: 900},
	vitestLane:      {most: 10, fewest: 3, spend: 200},
}

// selected is one test or test file of the plan, with what one run of it costs.
type selected struct {
	lane   lane
	group  string
	name   string
	cost   float64
	direct bool
}

// fit picks the repeat count for one lane and the tests that fit the budget at
// that count.
func fit(items []selected, b budget) (count int, kept, left []selected) {
	total := 0.0
	for _, item := range items {
		total += item.cost
	}
	if total == 0 {
		return b.most, items, nil
	}
	count = min(b.most, int(math.Floor(b.spend/total)))
	if count >= b.fewest {
		return count, items, nil
	}
	ordered := append([]selected(nil), items...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].direct && !ordered[j].direct })
	spent := 0.0
	for i, item := range ordered {
		if i > 0 && (spent+item.cost)*float64(b.fewest) > b.spend {
			left = append(left, item)
			continue
		}
		spent += item.cost
		kept = append(kept, item)
	}
	return b.fewest, kept, left
}

// vitestFile is a file the default vitest lane collects, by vitest's own
// include pattern. The census and Playwright trees under frontend/ are left
// out, because other lanes run them.
var vitestFile = regexp.MustCompile(`^frontend/(.+\.(test|spec)\.[cm]?[jt]sx?)$`)

func changedVitestFiles(root string, changes map[string]*fileChange) []selected {
	var found []selected
	for _, file := range sortedKeys(changes) {
		match := vitestFile.FindStringSubmatch(file)
		if match == nil || strings.Contains(file, "/node_modules/") ||
			strings.HasPrefix(file, "frontend/census/") || strings.HasPrefix(file, "frontend/e2e/") {
			continue
		}
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(file))); err != nil || info.IsDir() {
			continue
		}
		found = append(found, selected{lane: vitestLane, name: match[1], cost: 1, direct: true})
	}
	return found
}

// testTimes reads the lane's timing file: `dir|package|Test|seconds`, with
// `*` for the mean of a package's tests under one second.
func testTimes(file string) (map[string]float64, error) {
	times := map[string]float64{}
	if file == "" {
		return times, nil
	}
	raw, err := os.ReadFile(file) // #nosec G304 -- the lane's own timing file, named by the caller
	if err != nil {
		return nil, err
	}
	for line := range strings.Lines(string(raw)) {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) != 4 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		seconds, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			return nil, fmt.Errorf("%s: %q: %w", file, line, err)
		}
		times[strings.Join(fields[:3], "|")] = seconds
	}
	return times, nil
}

// unknownTestSeconds is the cost of an integration test the timing file does
// not list, which is most often a new test. The median of the lane is 2.5 s.
const unknownTestSeconds = 3.0

func integrationCost(times map[string]float64, dir, name string) float64 {
	pkg := "./" + strings.TrimPrefix(dir, "backend/")
	if dir == "backend" {
		pkg = "."
	}
	for _, key := range []string{"backend|" + pkg + "|" + name, "backend|" + pkg + "|*"} {
		if seconds, ok := times[key]; ok {
			return seconds
		}
	}
	return unknownTestSeconds
}

// writePlan prints one line per package or file: lane, count, target and, for
// Go, the -run pattern. What it leaves out goes to notes.
func writePlan(out, notes io.Writer, items []selected) error {
	for _, l := range []lane{unitLane, integrationLane, vitestLane} {
		var mine []selected
		for _, item := range items {
			if item.lane == l {
				mine = append(mine, item)
			}
		}
		if len(mine) == 0 {
			continue
		}
		count, kept, left := fit(mine, budgets[l])
		if _, err := fmt.Fprintf(notes, "%s: %d selected, each run %d times\n", l, len(kept), count); err != nil {
			return err
		}
		for _, item := range left {
			if _, err := fmt.Fprintf(notes, "%s: over the budget, left out: %s %s\n", l, item.group, item.name); err != nil {
				return err
			}
		}
		if err := writeLane(out, l, count, kept); err != nil {
			return err
		}
	}
	return nil
}

func writeLane(out io.Writer, l lane, count int, kept []selected) error {
	if l == vitestLane {
		for _, item := range kept {
			if _, err := fmt.Fprintf(out, "%s\t%d\t%s\n", l, count, item.name); err != nil {
				return err
			}
		}
		return nil
	}
	names := map[string][]string{}
	for _, item := range kept {
		names[item.group] = append(names[item.group], regexp.QuoteMeta(item.name))
	}
	for _, dir := range sortedKeys(names) {
		sort.Strings(names[dir])
		if _, err := fmt.Fprintf(out, "%s\t%d\t%s\t^(%s)$\n", l, count, dir, strings.Join(names[dir], "|")); err != nil {
			return err
		}
	}
	return nil
}
