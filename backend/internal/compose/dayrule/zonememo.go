// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dayrule

import (
	"context"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// zoneMemo holds the zones one read resolved, by surface and member. Every lane
// of that read then counts its day in the same zone from one query.
type zoneMemo struct {
	mu    sync.Mutex
	zones map[zoneMemoKey]*time.Location
}

type zoneMemoKey struct {
	surface occurredday.Surface
	user    ids.UUID
}

type zoneMemoContextKey struct{}

// WithZonesOncePerRead answers ctx with a memo of the zones it resolves. A
// read binds it at its start, so its lanes ask the database for a zone once.
func WithZonesOncePerRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, zoneMemoContextKey{}, &zoneMemo{zones: map[zoneMemoKey]*time.Location{}})
}

// memoKey names a zone the way Zone reads it: Combined is one zone whoever asks.
func memoKey(surface occurredday.Surface, user ids.UUID) zoneMemoKey {
	if surface != occurredday.Personal {
		user = ids.Nil
	}
	return zoneMemoKey{surface: surface, user: user}
}

// rememberedZone is the zone the read already resolved for surface and user.
func rememberedZone(ctx context.Context, surface occurredday.Surface, user ids.UUID) (*time.Location, bool) {
	memo, ok := ctx.Value(zoneMemoContextKey{}).(*zoneMemo)
	if !ok {
		return nil, false
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	zone, known := memo.zones[memoKey(surface, user)]
	return zone, known
}

// rememberZone keeps a resolved zone for the rest of the read. A failure is
// never kept, so the next lane asks again.
func rememberZone(ctx context.Context, surface occurredday.Surface, user ids.UUID, zone *time.Location) {
	memo, ok := ctx.Value(zoneMemoContextKey{}).(*zoneMemo)
	if !ok {
		return
	}
	memo.mu.Lock()
	defer memo.mu.Unlock()
	memo.zones[memoKey(surface, user)] = zone
}
