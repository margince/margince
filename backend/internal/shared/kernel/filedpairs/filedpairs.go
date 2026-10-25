// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package filedpairs lets a caller learn which review-queue pairs one write
// filed. The provider behind the write returns only a reference. So the filer
// notes each pair id on a sink the caller put in the context. A pair filed by
// any other writer never reaches that sink.
package filedpairs

import (
	"context"
	"sync"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type sinkKey struct{}

// Sink collects the pair ids filed under the context it was made for.
type Sink struct {
	mu  sync.Mutex
	ids []ids.UUID
}

// Collect returns a context whose filings are noted on the returned sink.
func Collect(ctx context.Context) (context.Context, *Sink) {
	sink := &Sink{}
	return context.WithValue(ctx, sinkKey{}, sink), sink
}

// Note records a pair id on the sink in ctx, and does nothing without one.
func Note(ctx context.Context, pair ids.UUID) {
	sink, ok := ctx.Value(sinkKey{}).(*Sink)
	if !ok {
		return
	}
	sink.mu.Lock()
	defer sink.mu.Unlock()
	sink.ids = append(sink.ids, pair)
}

// IDs returns the ids noted so far. A transaction retry can note an id whose
// row was rolled back, so a reader must confirm each against the queue.
func (s *Sink) IDs() []ids.UUID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ids.UUID(nil), s.ids...)
}
