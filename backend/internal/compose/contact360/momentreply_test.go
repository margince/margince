// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReplyRecommendationsKeepTheirEvidenceMessage(t *testing.T) {
	outbound := openapi_types.UUID(ids.NewV7())
	inboundID := openapi_types.UUID(ids.NewV7())
	for _, inbound := range []bool{false, true} {
		page := &crmcontracts.Contact360{LastOutboundAt: new(at(40)), LastOutboundActivityId: &outbound}
		if inbound {
			page.LastInboundAt, page.LastLiveInboundAt = new(at(1)), new(at(1))
			page.LastLiveInboundActivityId = &inboundID
		}
		want := outbound
		if inbound {
			want = inboundID
		}
		got := momentCopyFor("en").deriveMoment(readerCtx(), now, page)
		destination := got.RecommendedAction.Destination
		if destination == nil || destination.EntityId == nil || *destination.EntityId != want ||
			destination.EntityType == nil || *destination.EntityType != crmcontracts.ContactMomentDestinationEntityTypeActivity {
			t.Fatalf("recommendation dropped its message: %+v", destination)
		}
	}
}

func TestARecommendationWithoutItsOriginalMessageCannotDraft(t *testing.T) {
	page := &crmcontracts.Contact360{LastOutboundAt: new(at(40))}
	got := momentCopyFor("en").deriveMoment(readerCtx(), now, page)
	if got.RecommendedAction.State != crmcontracts.ContactMomentActionStateBlocked || got.RecommendedAction.BlockedReason == nil {
		t.Fatalf("a missing source must not turn into generic outreach: %+v", got.RecommendedAction)
	}
}
