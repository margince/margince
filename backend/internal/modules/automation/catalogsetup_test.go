// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"strings"
	"testing"
)

func TestEveryCatalogTemplateHasSummarySetupPreviewAndAnAction(t *testing.T) {
	for _, entry := range Catalog() {
		t.Run(entry.Key, func(t *testing.T) {
			if strings.TrimSpace(entry.Name) == "" || strings.TrimSpace(entry.Description) == "" {
				t.Fatal("a template needs a name and a summary")
			}
			if entry.Validate == nil {
				t.Fatal("a template needs parameter validation")
			}
			if required, ok := entry.ParamsSchema[schemaKeyRequired].([]string); ok && len(required) > 0 && validateRequiredSetup(entry, nil) == nil {
				t.Fatal("an incomplete required setup was accepted")
			}
			_, activity := activityScanHandlers[entry.Key]
			_, date := dateFieldScanHandlers[entry.Key]
			if strings.HasPrefix(entry.Trigger, "clock:") {
				if !activity && !date {
					t.Fatal("a scheduled template needs a population preview")
				}
			} else if !previewDefs()[entry.Key].eventOnly {
				t.Fatal("an event template needs a preview of future events")
			}
			if _, ok := ActionDefFor(ActionType(entry.Action)); !ok {
				t.Fatal("a template needs a registered action")
			}
		})
	}
}
