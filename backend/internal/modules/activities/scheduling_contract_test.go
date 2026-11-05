// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// schedulingProfileBlock is the SchedulingProfile schema as crm.yaml writes it:
// one flow-style line per property, up to the next schema.
var schedulingProfileBlock = regexp.MustCompile(`(?s)\n    SchedulingProfile:\n(.*?)\n    [A-Za-z]+:\n`)

// declaredNumber reads `key: N` out of one property's line of the schema.
func declaredNumber(t *testing.T, block, property, key string) int {
	t.Helper()
	line := regexp.MustCompile(`\n\s+` + property + `: \{([^\n]*)\}`).FindStringSubmatch(block)
	if line == nil {
		t.Fatalf("the contract has no %s property on SchedulingProfile", property)
	}
	number := regexp.MustCompile(key + `: (\d+)`).FindStringSubmatch(line[1])
	if number == nil {
		t.Fatalf("%s declares no %s", property, key)
	}
	value, err := strconv.Atoi(number[1])
	if err != nil {
		t.Fatalf("%s %s is not a number: %v", property, key, err)
	}
	return value
}

// The meeting-settings limits are enforced here and declared in crm.yaml. This
// reads the declaration, so a limit moved in one place and not the other fails.
func TestSchedulingLimitsAreTheOnesTheContractDeclares(t *testing.T) {
	raw, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	found := schedulingProfileBlock.FindStringSubmatch("\n" + string(raw))
	if found == nil {
		t.Fatal("the contract has no SchedulingProfile schema")
	}
	block := found[1]

	for _, r := range schedulingRanges {
		if lo, hi := declaredNumber(t, block, r.field, "minimum"), declaredNumber(t, block, r.field, "maximum"); lo != r.lo || hi != r.hi {
			t.Errorf("%s: enforced %d to %d, the contract declares %d to %d", r.field, r.lo, r.hi, lo, hi)
		}
	}
	for field, enforced := range map[string]int{
		"title": maxSchedulingTitle, "location": maxSchedulingLocation, "calendar_id": maxSchedulingCalendar,
	} {
		if declared := declaredNumber(t, block, field, "maxLength"); declared != enforced {
			t.Errorf("%s: enforced %d characters, the contract declares %d", field, enforced, declared)
		}
	}
	blocking := regexp.MustCompile(`blocking_calendars: \{[^\n]*items: \{[^}]*maxLength: (\d+)`).FindStringSubmatch(block)
	if blocking == nil || blocking[1] != strconv.Itoa(maxSchedulingCalendar) {
		t.Errorf("blocking_calendars: enforced %d characters per entry, the contract declares %v", maxSchedulingCalendar, strings.Join(blocking, " "))
	}
}
