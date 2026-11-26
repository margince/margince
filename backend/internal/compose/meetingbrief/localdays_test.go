// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/shared/kernel/datenotation"
)

// A header written on the installation's clock names the day the reader sees.
// Far east of UTC that is the next day, and the line still stands.
func TestAHeaderOnTheInstallationsClockIsGroundedFarFromUTC(t *testing.T) {
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	in := fullInput()
	in.Zone = kiritimati
	in.Dates = datenotation.Notation{Lang: "en", Zone: kiritimati}
	in.StartsAt = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	header := Sentence{Text: meetingLine(in), Evidence: []Evidence{{EntityType: citeActivity, EntityID: in.ActivityID}}}
	if reason := claims.Ungrounded(header, knownRecords(in)); reason != "" {
		t.Fatalf("the header %q was dropped: %s", header.Text, reason)
	}
	wrong := Sentence{Text: "Q3 review on 20/08/2026.", Evidence: header.Evidence}
	if claims.Grounded(wrong, knownRecords(in)) {
		t.Fatal("a day the meeting is not on was accepted")
	}
}
