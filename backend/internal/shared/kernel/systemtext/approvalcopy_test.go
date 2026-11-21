// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A receipt with no readable name still reads as a sentence: the booking names
// the contact as its subject, the mail as its object.
func TestATaskReceiptForAnUnnamedContactReadsInGerman(t *testing.T) {
	for booking, want := range map[bool]string{
		true:  "Erledigt, weil der Kontakt ein Meeting für 2026-11-21 gebucht hat",
		false: "Erledigt, weil du dem Kontakt am 2026-11-21 geschrieben hast",
	} {
		contact := Copy(TaskClosedContactKey(booking), textlang.German)
		if got := fmt.Sprintf(Copy(TaskClosedKey(booking), textlang.German), contact, "2026-11-21"); got != want {
			t.Errorf("booking=%v: %q, want %q", booking, got, want)
		}
	}
}
