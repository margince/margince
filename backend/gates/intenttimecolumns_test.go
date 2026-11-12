// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var localTimestampColumn = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*) timestamp without time zone\b`)

func TestEveryLocalTimestampHasACheckedTimezonePair(t *testing.T) {
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	catalog := string(raw)
	failures, count := localTimestampPairErrors(catalog)
	for _, failure := range failures {
		t.Error(failure)
	}
	if count == 0 {
		t.Fatal("the local-time census found no wall columns")
	}
}

func localTimestampPairErrors(catalog string) ([]string, int) {
	var failures []string
	count := 0
	for _, line := range strings.Split(catalog, "\n") {
		match := localTimestampColumn.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		count++
		table, column := match[1], match[2]
		if !strings.HasSuffix(column, "_wall") {
			failures = append(failures, fmt.Sprintf("%s.%s is an instant stored without a zone", table, column))
			continue
		}
		pattern := regexp.MustCompile(`^public\.` + regexp.QuoteMeta(table) + `\.[a-z_][a-z0-9_]* CHECK \((?:public\.)?valid_intent_time\(` + regexp.QuoteMeta(column) + `, ([a-z_][a-z0-9_]*)\)\)(?: NOT VALID)?$`)
		paired := false
		for _, constraint := range strings.Split(catalog, "\n") {
			pair := pattern.FindStringSubmatch(constraint)
			if pair == nil {
				continue
			}
			zone := regexp.MustCompile(`(?m)^public\.` + regexp.QuoteMeta(table) + `\.` + regexp.QuoteMeta(pair[1]) + ` text\b`)
			paired = zone.MatchString(catalog)
			if paired {
				break
			}
		}
		if !paired {
			failures = append(failures, fmt.Sprintf("%s.%s has no checked wall-time and timezone pair", table, column))
		}
	}
	return failures, count
}

func TestTheLocalTimeCensusRejectsAnIneffectivePairCheck(t *testing.T) {
	columns := "public.activity.due_wall timestamp without time zone gen=- def=-\npublic.activity.due_zone text gen=- def=-\n"
	for _, check := range []string{
		"CHECK (valid_intent_time(due_wall, due_zone))",
		"CHECK (public.valid_intent_time(due_wall, due_zone)) NOT VALID",
	} {
		failures, count := localTimestampPairErrors(columns + "public.activity.pair " + check)
		if count != 1 || len(failures) != 0 {
			t.Errorf("valid pair %q: count=%d failures=%v", check, count, failures)
		}
	}
	for _, check := range []string{
		"CHECK (valid_intent_time(due_wall, due_zone) OR true)",
		"CHECK (valid_intent_time(due_wall, missing_zone))",
		"CHECK (due_wall IS NULL)",
	} {
		failures, count := localTimestampPairErrors(columns + "public.activity.pair " + check)
		if count != 1 || len(failures) != 1 {
			t.Errorf("ineffective pair %q: count=%d failures=%v", check, count, failures)
		}
	}
}
