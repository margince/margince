// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"errors"
	"testing"
)

func validEntry() Entry {
	return Entry{
		Slot: SlotContactHeader, Label: "extVisitLog.open", Icon: IconClipboardList,
		Target: "report", Object: "ext_visit_log_report",
	}
}

func TestEntryValidateAdmitsAWholeDeclaration(t *testing.T) {
	if err := validEntry().Validate("visit-log"); err != nil {
		t.Fatalf("a whole entry was refused: %v", err)
	}
	screenItself := validEntry()
	screenItself.Target = ""
	if err := screenItself.Validate("visit-log"); err != nil {
		t.Fatalf("an entry opening the screen itself was refused: %v", err)
	}
}

func TestEntryValidateRefusesWhatNoCoreScreenCanRender(t *testing.T) {
	for name, edit := range map[string]func(*Entry){
		"an unknown slot":         func(e *Entry) { e.Slot = "record_header.lead" },
		"no slot":                 func(e *Entry) { e.Slot = "" },
		"an unknown icon":         func(e *Entry) { e.Icon = "rocket" },
		"a label that is text":    func(e *Entry) { e.Label = "Log a visit" },
		"a target with a slash":   func(e *Entry) { e.Target = "a/b" },
		"a core object":           func(e *Entry) { e.Object = "contact" },
		"another unit's object":   func(e *Entry) { e.Object = "ext_visit_report" },
		"the bare unit namespace": func(e *Entry) { e.Object = "ext_visit_log" },
	} {
		t.Run(name, func(t *testing.T) {
			entry := validEntry()
			edit(&entry)
			if err := entry.Validate("visit-log"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
