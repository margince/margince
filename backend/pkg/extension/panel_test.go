// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"errors"
	"testing"
)

func validPanel() Panel {
	return Panel{Slot: SlotDealPanel, Export: "VisitPanel", Object: "ext_visit_log_report"}
}

func TestPanelValidateAdmitsAWholeDeclaration(t *testing.T) {
	if err := validPanel().Validate("visit-log"); err != nil {
		t.Fatalf("a whole panel was refused: %v", err)
	}
}

func TestPanelValidateRefusesWhatNoCorePageCanDraw(t *testing.T) {
	for name, edit := range map[string]func(*Panel){
		"an entry slot":           func(p *Panel) { p.Slot = "record_header.deal" },
		"no slot":                 func(p *Panel) { p.Slot = "" },
		"no export":               func(p *Panel) { p.Export = "" },
		"a lower-case export":     func(p *Panel) { p.Export = "visitPanel" },
		"the default export":      func(p *Panel) { p.Export = "default" },
		"an export with a dot":    func(p *Panel) { p.Export = "Visit.Panel" },
		"a core object":           func(p *Panel) { p.Object = "deal" },
		"another unit's object":   func(p *Panel) { p.Object = "ext_visit_report" },
		"the bare unit namespace": func(p *Panel) { p.Object = "ext_visit_log" },
	} {
		t.Run(name, func(t *testing.T) {
			panel := validPanel()
			edit(&panel)
			if err := panel.Validate("visit-log"); !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}
