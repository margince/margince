// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

func visitEntry(slot extension.EntrySlot, target string) extension.Entry {
	return extension.Entry{
		Slot: slot, Label: "extVisits.open", Icon: extension.IconMapPin,
		Target: target, Object: "ext_visits_report",
	}
}

func unitWithEntries(entries ...extension.Entry) extension.Extension {
	return extension.Extension{Name: "visits", Version: "0.1.0", Entries: entries}
}

func TestPreflightEntriesAdmitsOneViewPerSlot(t *testing.T) {
	unit := unitWithEntries(
		visitEntry(extension.SlotLogActivity, ""),
		visitEntry(extension.SlotLogActivity, "checklist"),
		visitEntry(extension.SlotDealHeader, ""),
	)
	if err := preflightEntries(unit); err != nil {
		t.Fatalf("distinct entries were refused: %v", err)
	}
}

func TestPreflightEntriesRefusesOneViewTwiceInOneSlot(t *testing.T) {
	unit := unitWithEntries(visitEntry(extension.SlotLogActivity, ""), visitEntry(extension.SlotLogActivity, ""))
	if err := preflightEntries(unit); err == nil || !strings.Contains(err.Error(), "two entries") {
		t.Fatalf("err = %v, want the duplicate-entry refusal", err)
	}
}

func TestPreflightEntriesRefusesASlotNoCoreScreenRenders(t *testing.T) {
	if err := preflightEntries(unitWithEntries(visitEntry("record_header.lead", ""))); err == nil {
		t.Fatal("an entry in a slot no core screen renders was admitted")
	}
}

func TestPreflightSettingsRefusesAKeyTwiceAndAnUndrawableSetting(t *testing.T) {
	live := extension.Setting{Key: "live", Label: "Live", Kind: extension.SettingSwitch}
	unit := func(s ...extension.Setting) extension.Extension {
		return extension.Extension{Name: "visits", Version: "0.1.0", Settings: s}
	}
	if err := preflightSettings(unit(live, extension.Setting{Key: "region", Label: "Region", Kind: extension.SettingText})); err != nil {
		t.Fatalf("distinct settings were refused: %v", err)
	}
	if err := preflightSettings(unit(live, live)); err == nil || !strings.Contains(err.Error(), "twice") {
		t.Errorf("err = %v, want the duplicate-key refusal", err)
	}
	if err := preflightSettings(unit(extension.Setting{Key: "live", Kind: extension.SettingSwitch})); err == nil {
		t.Error("a setting with no label was admitted")
	}
}

func TestPreflightEntryObjectsNeedsAnOperationGatedOnTheObject(t *testing.T) {
	units := []extension.Extension{unitWithEntries(visitEntry(extension.SlotLogActivity, ""))}
	gated := []extension.Verb{{Unit: "visits", RbacObject: "ext_visits_report"}}
	if err := preflightEntryObjects(units, gated); err != nil {
		t.Fatalf("an entry on a gated object was refused: %v", err)
	}
	other := []extension.Verb{{Unit: "visits", RbacObject: "ext_visits_note"}}
	if err := preflightEntryObjects(units, other); err == nil {
		t.Fatal("an entry on an object no operation gates on was admitted")
	}
	// The object must be gated by the unit's own operation, not a namesake's.
	foreign := []extension.Verb{{Unit: "notes", RbacObject: "ext_visits_report"}}
	if err := preflightEntryObjects(units, foreign); err == nil {
		t.Fatal("an entry gated through another unit's operation was admitted")
	}
}
