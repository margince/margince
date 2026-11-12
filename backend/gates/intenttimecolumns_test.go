// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
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
	count := 0
	for _, line := range strings.Split(catalog, "\n") {
		match := localTimestampColumn.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		count++
		table, column := match[1], match[2]
		if !strings.HasSuffix(column, "_wall") {
			t.Errorf("%s.%s is an instant stored without a zone", table, column)
			continue
		}
		pattern := regexp.MustCompile(`^public\.` + regexp.QuoteMeta(table) + `\.[a-z_][a-z0-9_]* CHECK .*valid_intent_time\(` + regexp.QuoteMeta(column) + `, ([a-z_][a-z0-9_]*)\)`)
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
			t.Errorf("%s.%s has no checked wall-time and timezone pair", table, column)
		}
	}
	if count == 0 {
		t.Fatal("the local-time census found no wall columns")
	}
}
