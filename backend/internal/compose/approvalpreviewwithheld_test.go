// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
)

func TestThePromotionPreviewNamesHowManyActivitiesMoveUnread(t *testing.T) {
	carry := contacts.LeadPromotionCarry{WithheldActivities: 2}
	for sourceOnly, field := range map[bool]string{false: "promotion_move_withheld_activities", true: "creation_source_withheld_activities"} {
		details, err := approvalPreviewReader{}.promotionCarryDetails(context.Background(), carry, sourceOnly)
		if err != nil {
			t.Fatal(err)
		}
		if len(details) != 1 || details[0].Field != field || details[0].Value.Text != "2" {
			t.Errorf("sourceOnly=%t: details %+v, want one %s detail counting 2", sourceOnly, details, field)
		}
	}
	none, err := approvalPreviewReader{}.promotionCarryDetails(context.Background(), contacts.LeadPromotionCarry{}, false)
	if err != nil || len(none) != 0 {
		t.Errorf("a carry with nothing withheld renders %+v, %v; want no detail", none, err)
	}
}
