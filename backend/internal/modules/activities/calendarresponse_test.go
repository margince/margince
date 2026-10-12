// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarmethod"
)

// The predicate and the prompt name the same methods as a calendar response.
// Otherwise the model reads a message as an answer while the lane still owes it.
func TestTheCalendarResponseReadersAgree(t *testing.T) {
	t.Parallel()
	inSQL := map[string]bool{}
	for _, quoted := range regexp.MustCompile(`'([A-Z]+)'`).FindAllStringSubmatch(calendarResponseSQL, -1) {
		inSQL[quoted[1]] = true
	}
	if len(inSQL) == 0 {
		t.Fatal("read no method out of calendarResponseSQL")
	}
	for _, method := range []string{"PUBLISH", "REQUEST", "REPLY", "ADD", "CANCEL", "REFRESH", "COUNTER", "DECLINECOUNTER", ""} {
		if inSQL[method] != calendarmethod.IsResponse(method) {
			t.Errorf("%q: calendarResponseSQL says %v, calendarmethod.IsResponse says %v", method, inSQL[method], calendarmethod.IsResponse(method))
		}
	}
}

// The re-read reads mail older than CalendarMethodMigration, so it must name
// the migration that adds the column; another would draw the line elsewhere.
func TestTheCalendarMethodMigrationAddsTheColumn(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("../../../migrations/core/" + CalendarMethodMigration + "_*.up.sql")
	if err != nil || len(files) != 1 {
		t.Fatalf("CalendarMethodMigration %s names %d migrations (%v)", CalendarMethodMigration, len(files), err)
	}
	body, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ADD COLUMN calendar_method") {
		t.Errorf("%s does not add activity.calendar_method", files[0])
	}
}
