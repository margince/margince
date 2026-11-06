// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The guard names a hand-recorded exchange by id, so a rep can take it back
// from where it shows. A derived exchange has no id and the wire carries none.
func TestTheGuardNamesARecordedExchangeByID(t *testing.T) {
	t.Parallel()
	id := ids.NewV7()
	recorded, err := wireQualifying(QualifyingEvent{
		ID: id.String(), Kind: KindRequestedBySubject, OccurredAt: time.Unix(0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if recorded.Id == nil || ids.UUID(*recorded.Id) != id {
		t.Errorf("recorded exchange id = %v, want %s", recorded.Id, id)
	}
	derived, err := wireQualifying(QualifyingEvent{Kind: "inbound_message", OccurredAt: time.Unix(0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if derived.Id != nil {
		t.Errorf("derived exchange id = %v, want none", *derived.Id)
	}
}
