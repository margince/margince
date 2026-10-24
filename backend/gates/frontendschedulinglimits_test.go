// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The Meetings settings screen stops a title or a location at the contract's
// length before the request is sent. The two lengths are a declared mirror of
// crm.yaml's SchedulingProfile, not a second answer. A limit changed on one side
// and not the other fails here.

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

const meetingSettingsSections = "../frontend/src/features/booking/meeting-link-drawer.tsx"

func TestTheMeetingSettingsScreenStopsTextAtTheContractsLengths(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(meetingSettingsSections)
	if err != nil {
		t.Fatalf("reading the meeting link drawer: %v", err)
	}
	doc := loadContract(t)
	schema, ok := resolveSchemaRef(doc, "#/components/schemas/SchedulingProfile")
	if !ok {
		t.Fatal("the contract has no SchedulingProfile")
	}
	properties, _ := schema["properties"].(map[string]any)

	for constant, field := range map[string]string{"TITLE_MAX": "title", "LOCATION_MAX": "location"} {
		match := regexp.MustCompile(`const ` + constant + ` = (\d+);`).FindSubmatch(source)
		if match == nil {
			t.Errorf("%s is gone from the screen; this gate needs updating with whatever replaced it", constant)
			continue
		}
		shown, _ := strconv.Atoi(string(match[1]))
		declared, _ := properties[field].(map[string]any)
		if got, _ := declared["maxLength"].(int); got != shown {
			t.Errorf("%s stops %s at %d characters, the contract declares %v", constant, field, shown, declared["maxLength"])
		}
	}
}
