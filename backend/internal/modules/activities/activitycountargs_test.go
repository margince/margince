// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// A count has no ORDER BY, so every argument its filter binds must be named in
// the `where` terms or the content clause. An argument the statement never
// mentions is refused by Postgres ("could not determine data type").
func TestTheCountFilterBindsNoArgumentItNeverMentions(t *testing.T) {
	until := time.Date(2026, 6, 16, 17, 0, 0, 0, time.UTC)
	asOf := time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)
	in := ListActivitiesInput{
		OpenAndDueBy: &until, WorklistAsOf: &asOf, DayZone: "Asia/Ho_Chi_Minh", Sort: new("due_at"), countOnly: true,
	}
	where, content, _, args, err := listActivitiesFilter(unscopedCtx(), in)
	if err != nil {
		t.Fatal(err)
	}
	statement := strings.Join(where, " AND ") + " " + content
	for position := 1; position <= len(args); position++ {
		if !regexp.MustCompile(fmt.Sprintf(`\$%d\b`, position)).MatchString(statement) {
			t.Errorf("argument $%d (%v) is bound but the count statement never mentions it", position, args[position-1])
		}
	}
	if !strings.Contains(statement, "::text") {
		t.Error("the list's own zone is not in the statement as a typed parameter")
	}
}
