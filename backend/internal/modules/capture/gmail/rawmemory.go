// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"

	"golang.org/x/sync/semaphore"
)

const (
	// The capped response buffer, JSON string and decoded bytes coexist during decoding.
	rawFetchReservation    = 3 * maxRawMessageBytes
	rawProcessBudget       = 2 * rawFetchReservation
	headerFetchReservation = 4 * maxJSONResponseBytes
)

var processRawMemory = semaphore.NewWeighted(rawProcessBudget)

type rawMemoryKey struct{}

type rawReservation struct {
	budget *semaphore.Weighted
	held   int64
}

func reserveMemory(ctx context.Context, budget *semaphore.Weighted, bytes int64) (*rawReservation, error) {
	if budget == nil {
		return nil, nil //nolint:nilnil // header walks do not retain raw mail
	}
	if err := budget.Acquire(ctx, bytes); err != nil {
		return nil, err
	}
	return &rawReservation{budget: budget, held: bytes}, nil
}

func (r *rawReservation) context(ctx context.Context) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, rawMemoryKey{}, r)
}

func (r *rawReservation) shrink(bytes int64) {
	if r != nil && bytes < r.held {
		r.budget.Release(r.held - bytes)
		r.held = bytes
	}
}

func (r *rawReservation) release() {
	if r != nil {
		r.shrink(0)
	}
}

// A declared length is useful only when the reader enforces that same limit.
func reserveResponse(ctx context.Context, length int64) {
	if reservation, ok := ctx.Value(rawMemoryKey{}).(*rawReservation); ok {
		reservation.shrink(max(4096, 4*length))
	}
}

func retainedRawBytes(msg Message) int64 {
	bytes := int64(cap(msg.RFC822) + cap(msg.Labels)*24 + 128)
	for _, label := range msg.Labels {
		bytes += int64(len(label))
	}
	return bytes
}
