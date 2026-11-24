// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNoConfirmedSourceIsAbsentFromThePage(t *testing.T) {
	if got := confirmedSourceWire(nil); got != nil {
		t.Fatalf("a contact with no confirmed source reads %+v, want absent", got)
	}
}

func TestTheConfirmedSourceNamesItsConfirmerWithoutTheCredential(t *testing.T) {
	passport := ids.NewV7()
	confirmedAt := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	wire := confirmedSourceWire(&consent.ConfirmedNoticeSource{
		CaseID: ids.NewV7(), Kind: "in_person_permission", ConfirmedAt: confirmedAt,
		ConfirmedBy: &auditattribution.Attribution{ActorType: "agent", ActorID: "agent:x", PassportID: &passport},
	})
	if wire == nil || wire.SourceKind != "in_person_permission" || !wire.ConfirmedAt.Equal(confirmedAt) || wire.SourceObtainedAt != nil {
		t.Fatalf("the confirmed source reads %+v", wire)
	}
	if wire.ConfirmedBy == nil || wire.ConfirmedBy.ActorId != "agent:x" || wire.ConfirmedBy.PassportId != nil {
		t.Fatalf("the confirmer reads %+v, want the actor without its passport id", wire.ConfirmedBy)
	}
}
