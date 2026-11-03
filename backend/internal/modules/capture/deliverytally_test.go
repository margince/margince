// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestOnlyEmailRecordsCountAsDelivered(t *testing.T) {
	ctx, tally := withDeliveryTally(context.Background())
	noteDelivered(ctx, connector.NormalizedRecord{Fields: ActivityFields{Kind: meetingKind}, ProviderReceivedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)})
	if tally.receivedAt() != nil {
		t.Fatal("a calendar event counted as mail")
	}
	noteDelivered(ctx, connector.NormalizedRecord{Fields: ActivityFields{Kind: kindEmail}})
	if tally.receivedAt() != nil {
		t.Fatal("an unrelated record counted as mail")
	}
	noteDelivered(ctx, connector.NormalizedRecord{Fields: ActivityFields{Kind: kindEmail}, ProviderReceivedAt: time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)})
	if tally.receivedAt() == nil {
		t.Fatal("email delivery was not observed")
	}
}

func TestDeliveryTallyKeepsProviderArrivalInsteadOfReplayTime(t *testing.T) {
	ctx, tally := withDeliveryTally(context.Background())
	first := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	rec := connector.NormalizedRecord{Fields: ActivityFields{Kind: kindEmail}, ProviderReceivedAt: first}
	noteDelivered(ctx, rec)
	noteDelivered(ctx, rec)
	rec.ProviderReceivedAt = first.Add(-time.Hour)
	noteDelivered(ctx, rec)
	if got := tally.receivedAt(); got == nil || !got.Equal(first) {
		t.Fatalf("provider arrival = %v", got)
	}
	rec.ProviderReceivedAt = time.Time{}
	noteDelivered(ctx, rec)
	if got := tally.receivedAt(); got == nil || !got.Equal(first) {
		t.Fatalf("unknown arrival moved delivery = %v", got)
	}
}
