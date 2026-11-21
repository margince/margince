// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

func TestNoStarterShipsOnWhileItNeedsSetup(t *testing.T) {
	for _, entry := range Catalog() {
		if !entry.Seeded {
			continue
		}
		t.Run(entry.Key, func(t *testing.T) {
			seeded := Automation{Key: entry.Key, Params: json.RawMessage(`{}`)}
			state := configurationOf(seeded, false).State
			if state == crmcontracts.AutomationSetupStateNeedsSetup && seededEnabled(entry) {
				t.Fatal("a starter that needs setup must be seeded paused")
			}
		})
	}
}

func TestSeededStartersAreOnOnlyWhenSetupIsComplete(t *testing.T) {
	if seededEnabled(mustEntry(t, renewalReminderName)) {
		t.Fatal("the renewal starter needs a record type and a date field, so it must start paused")
	}
	if !seededEnabled(mustEntry(t, routeLeadName)) {
		t.Fatal("a starter with no required setting must start on")
	}
}

func mustEntry(t *testing.T, key string) CatalogEntry {
	t.Helper()
	entry, ok := CatalogEntryByKey(key)
	if !ok {
		t.Fatalf("catalog has no %s", key)
	}
	return entry
}

func TestEnabledRuleThatNeedsSetupIsReportedPausedAndNotRun(t *testing.T) {
	record := Automation{Key: renewalReminderName, Enabled: true, Params: json.RawMessage(`{}`)}
	state := configurationOf(record, false)
	record.Configuration = &state
	wire, err := WireAutomation(record)
	if err != nil {
		t.Fatal(err)
	}
	if wire.Status != crmcontracts.AutomationStatusPaused || *wire.SetupState != crmcontracts.AutomationSetupStateNeedsSetup {
		t.Fatalf("status=%s state=%v, want paused/needs_setup", wire.Status, *wire.SetupState)
	}
	if runnable, err := setupComplete(renewalReminderName, record.Params); err != nil || runnable {
		t.Fatalf("runnable=%v err=%v, want not runnable", runnable, err)
	}
}
