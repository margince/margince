// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
)

// ZoneUnresolvedAction is the system_log action of a run that counted its days
// in UTC, because the stored installation zone did not load.
const ZoneUnresolvedAction = "installation_zone.unresolved"

// FallbackZone is the zone a day is counted in when the stored one will not load.
const FallbackZone = "UTC"

type zoneWatchKey struct{}

// ZoneWatch collects the installation zone names one run could not load.
// A run reads its zone from several goroutines, so the list is locked.
type ZoneWatch struct {
	mu    sync.Mutex
	names []string
}

// WatchZones starts a watch for one run. EffectiveZone reports to it through
// the returned context.
func WatchZones(ctx context.Context) (context.Context, *ZoneWatch) {
	watch := &ZoneWatch{}
	return context.WithValue(ctx, zoneWatchKey{}, watch), watch
}

// EffectiveZone is the zone name a reader counts the installation's days in.
// Inside a watched background pass, a stored name Go cannot load reads as UTC.
//
// Outside a watch the stored name is returned as it is, so an interactive read
// still fails on it. One answer that mixed UTC with SQL's stored zone would be
// wrong with no sign.
func EffectiveZone(ctx context.Context, stored string) string {
	watch, watched := ctx.Value(zoneWatchKey{}).(*ZoneWatch)
	if !watched || ZoneLoads(stored) {
		return stored
	}
	watch.note(stored)
	return FallbackZone
}

// ZoneLoads reports whether Go's zone data holds the name.
func ZoneLoads(name string) bool {
	_, err := time.LoadLocation(name)
	return err == nil
}

func (w *ZoneWatch) note(name string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !slices.Contains(w.names, name) {
		w.names = append(w.names, name)
	}
}

// Unloaded lists the names the run could not load, in the order it met them.
func (w *ZoneWatch) Unloaded() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return slices.Clone(w.names)
}

// RecordTx writes one fault for the whole run, and nothing when every name
// loaded. A run reads its zone once per row, and one row per read would bury the log.
func (w *ZoneWatch) RecordTx(ctx context.Context, tx pgx.Tx) error {
	names := w.Unloaded()
	if len(names) == 0 {
		return nil
	}
	_, err := LogSystem(ctx, tx, ZoneUnresolvedAction, map[string]any{"timezones": names, "applied": FallbackZone})
	return err
}
