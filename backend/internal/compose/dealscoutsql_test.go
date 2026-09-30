// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The scout's read is bounded by the companies the cap keeps: the evidence it
// hands back and every document reading it looks up come only from `chosen`,
// and the cap is applied to companies that already qualify.
func TestTheScoutReadsOnlyTheCompaniesItChose(t *testing.T) {
	const companyCap = 7
	query, args := dealScoutSQL(time.Now().Add(-time.Hour), time.Now(), companyCap)
	chosen := strings.Index(query, "chosen AS (")
	cited := strings.Index(query, "cited AS (")
	final := strings.LastIndex(query, "FROM cited")
	if chosen < 0 || cited < chosen || final < cited {
		t.Fatalf("the read no longer chooses companies, then cites, then selects from the cited rows:\n%s", query)
	}
	if !strings.Contains(query[cited:final], "JOIN chosen USING (company_id)") {
		t.Fatal("the cited evidence is not narrowed to the chosen companies")
	}
	if n := strings.Count(query, "attachment_extraction"); n != 1 || strings.Index(query, "attachment_extraction") < final {
		t.Fatal("a document reading is looked up outside the final select over the chosen companies' evidence")
	}
	chosenBody := query[chosen:cited]
	for _, rule := range []string{"FROM ev WHERE kind <> 'signal'", "FROM pairs"} {
		if !strings.Contains(chosenBody, rule) {
			t.Fatalf("the cap chooses among companies without %q qualifying them first", rule)
		}
	}
	limit := regexp.MustCompile(`LIMIT \$(\d+)`).FindStringSubmatch(chosenBody)
	if limit == nil {
		t.Fatal("the chosen companies are not capped")
	}
	pos, err := strconv.Atoi(limit[1])
	if err != nil || args[pos-1] != companyCap {
		t.Fatalf("the cap binds %v, want %d", args[pos-1], companyCap)
	}
}
