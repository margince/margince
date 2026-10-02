// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// The two settings the sync owns: its switch, and the record of its last run.
const (
	PriceSyncKey        = "ai.price_sync"
	PriceSyncLastRunKey = "ai.price_sync_last_run"
)

// PriceSyncConfig is whether the daily job syncs model prices.
type PriceSyncConfig struct {
	AutoSync bool `json:"auto_sync"`
}

// PriceSyncSettings is on by default, sovereign included: reading two public
// price lists sends nothing of the installation's.
var PriceSyncSettings = settings.Define(PriceSyncKey, "ai_model_rate", "update", PriceSyncConfig{AutoSync: true}, nil)

// PriceSyncTrigger is who started a run.
type PriceSyncTrigger string

// A run is an admin's Refresh now or the daily sweep's.
const (
	PriceSyncManual    PriceSyncTrigger = "manual"
	PriceSyncScheduled PriceSyncTrigger = "scheduled"
)

// LastPriceSync is the most recent run, written only by PriceSync in the run's
// own transaction; nil until the first run.
type LastPriceSync struct {
	RanAt   time.Time         `json:"ran_at"`
	Trigger PriceSyncTrigger  `json:"trigger"`
	Report  RateRefreshReport `json:"report"`
}

// PriceSyncLastRun holds the last run; nil until the sync has run once.
var PriceSyncLastRun = settings.Define[*LastPriceSync](PriceSyncLastRunKey, "ai_model_rate", "update", nil, nil)
