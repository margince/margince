// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"context"

	"golang.org/x/sync/semaphore"

	"github.com/margince/margince/backend/internal/modules/capture/orderedfetch"
)

const (
	// The capped response buffer, JSON string and decoded bytes coexist during decoding.
	rawFetchReservation    = 3 * maxRawMessageBytes
	rawProcessBudget       = 2 * rawFetchReservation
	headerFetchReservation = 4 * maxJSONResponseBytes
)

var processRawMemory = semaphore.NewWeighted(rawProcessBudget)

// A declared length is useful only when the reader enforces that same limit.
func reserveResponse(ctx context.Context, length int64) {
	if reservation, ok := orderedfetch.FromContext(ctx); ok {
		reservation.Shrink(max(4096, 4*length))
	}
}

func retainedRawBytes(msg Message) int64 {
	bytes := int64(cap(msg.RFC822) + cap(msg.Labels)*24 + 128)
	for _, label := range msg.Labels {
		bytes += int64(len(label))
	}
	return bytes
}
