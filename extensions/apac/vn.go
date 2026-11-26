// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apac

import (
	"time"

	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	"github.com/margince/margince/backend/pkg/extension/messaging"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

// vietnameseSubjectRights are the Decree 356/2025/ND-CP Art. 5 deadlines.
// Every request is owed a first answer in 2 working days. It is carried out in
// 10 days to see or correct (Art. 5.3), 20 to erase (Art. 5.4) and 15 to
// restrict (Art. 5.2), counted in calendar days.
func vietnameseSubjectRights() subjectrights.Rules {
	return subjectrights.Rules{
		Jurisdiction: "vn",
		Version:      2,
		Instruments: []messaging.Instrument{
			{Name: "Decree 356/2025/ND-CP, Art. 5", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		Deadlines: map[subjectrights.RequestKind]subjectrights.Deadline{
			subjectrights.Access:      {Period: jurisdiction.Period{Days: 10}},
			subjectrights.Rectify:     {Period: jurisdiction.Period{Days: 10}},
			subjectrights.Erasure:     {Period: jurisdiction.Period{Days: 20}},
			subjectrights.Restriction: {Period: jurisdiction.Period{Days: 15}},
		},
		ReplyWithinWorkingDays: 2,
		// Working days are Vietnamese days.
		UTCOffset: 7 * time.Hour,
	}
}
