// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// The meeting-settings limits are enforced here and declared in crm.yaml. This
// reads the declaration, so a limit moved in one place and not the other fails.
func TestSchedulingLimitsAreTheOnesTheContractDeclares(t *testing.T) {
	raw, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Components struct {
			Schemas struct {
				SchedulingProfile struct {
					Properties map[string]struct {
						Minimum   *int `yaml:"minimum"`
						Maximum   *int `yaml:"maximum"`
						MaxLength *int `yaml:"maxLength"`
						Items     *struct {
							MaxLength *int `yaml:"maxLength"`
						} `yaml:"items"`
					} `yaml:"properties"`
				} `yaml:"SchedulingProfile"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	props := doc.Components.Schemas.SchedulingProfile.Properties

	for _, r := range schedulingRanges {
		declared, ok := props[r.field]
		if !ok || declared.Minimum == nil || declared.Maximum == nil {
			t.Errorf("%s: the contract declares no range", r.field)
			continue
		}
		if *declared.Minimum != r.lo || *declared.Maximum != r.hi {
			t.Errorf("%s: enforced %d to %d, the contract declares %d to %d", r.field, r.lo, r.hi, *declared.Minimum, *declared.Maximum)
		}
	}
	for field, enforced := range map[string]int{
		"title": maxSchedulingTitle, "location": maxSchedulingLocation, "calendar_id": maxSchedulingCalendar,
	} {
		if declared := props[field].MaxLength; declared == nil || *declared != enforced {
			t.Errorf("%s: enforced %d characters, the contract declares %v", field, enforced, declared)
		}
	}
	if items := props["blocking_calendars"].Items; items == nil || items.MaxLength == nil || *items.MaxLength != maxSchedulingCalendar {
		t.Errorf("blocking_calendars: enforced %d characters per entry, the contract declares %+v", maxSchedulingCalendar, items)
	}
}
