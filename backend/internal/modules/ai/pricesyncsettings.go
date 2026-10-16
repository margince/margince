// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"time"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// The two settings the sync owns: its switch, and the record of its last run.
const (
	PriceSyncKey        = "ai.price_sync"
	PriceSyncLastRunKey = "ai.price_sync_last_run"
	modelRateObject     = "ai_model_rate"
)

// PriceSyncConfig is whether the daily job syncs model prices.
type PriceSyncConfig struct {
	AutoSync bool `json:"auto_sync" yaml:"auto_sync"`
}

// PriceSyncSettings is on by default, sovereign included: reading two public
// price lists sends nothing of the installation's.
var PriceSyncSettings = config.DefineStored(config.Spec[PriceSyncConfig]{
	Key:     "ai.price_sync",
	Env:     "MARGINCE_AI_PRICE_SYNC",
	Type:    config.Struct[PriceSyncConfig](),
	Default: config.Some(PriceSyncConfig{AutoSync: true}),
	Doc:     "Whether the daily job syncs model prices, as {\"auto_sync\": true|false}. Refresh now runs either way.",
}, config.Store{RBACObject: modelRateObject, AuditVerb: settingsAuditVerb})

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

// PriceSyncLastRun holds the last run; nil until the sync has run once. Only
// the sync writes it, so it is a record, not configuration a boot source sets.
var PriceSyncLastRun = settings.Define[*LastPriceSync](PriceSyncLastRunKey, modelRateObject, settingsAuditVerb, nil, nil)
