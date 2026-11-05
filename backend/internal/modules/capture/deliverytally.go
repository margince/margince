// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type deliveryTallyKey struct{}

type deliveryTally struct {
	mu     sync.Mutex
	latest time.Time
}

func withDeliveryTally(ctx context.Context) (context.Context, *deliveryTally) {
	tally := &deliveryTally{}
	return context.WithValue(ctx, deliveryTallyKey{}, tally), tally
}

func noteDelivered(ctx context.Context, rec connector.NormalizedRecord) {
	tally, ok := ctx.Value(deliveryTallyKey{}).(*deliveryTally)
	if !ok {
		return
	}
	fields, ok := rec.Fields.(ActivityFields)
	if !ok || fields.Kind != kindEmail || rec.ProviderReceivedAt.IsZero() {
		return
	}
	tally.mu.Lock()
	defer tally.mu.Unlock()
	if rec.ProviderReceivedAt.After(tally.latest) {
		tally.latest = rec.ProviderReceivedAt
	}
}

// Provider arrival time stays fixed on replay, unlike the time of a sync.
func (t *deliveryTally) receivedAt() *time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.latest.IsZero() {
		return nil
	}
	latest := t.latest
	return &latest
}
