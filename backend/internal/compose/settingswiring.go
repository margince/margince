// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The settings catalog is assembled HERE, the way every other cross-module
// edge is (ADR-0054): each module declares the settings it owns, compose
// concatenates them into the one registry every store reads. platform/settings
// owns the mechanism and knows no domain; the modules own the meaning.

import (
	"encoding/json"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// settingsDefinitions is the whole catalog, in one list: the configuration
// registry's stored settings, then the settings.Define exceptions.
//
// This function is also the fitness gate's input: settingscatalog_test.go
// asserts uniqueness and module-prefix ownership over exactly this list, so a
// setting that is declared but never registered fails the same way a
// misprefixed one does.
var settingsDefinitions = sync.OnceValue(func() []settings.Definition {
	// The one cross-module edge in the catalog: identity owns
	// installation.base_currency because it owns the installation, but the
	// predicate that freezes it — a deal having stamped a conversion rate —
	// is the deals module's business, and identity may not read that table.
	// Injected here, once, the way ADR-0054 requires; settingscatalog_test.go
	// asserts it actually happened, because an unwired probe fails OPEN (the
	// setting stays changeable) rather than loudly.
	identity.LockBaseCurrencyWith(deals.BaseCurrencyLock(identity.BaseCurrency.Key()))

	return slices.Concat(storedDefinitions(), defineExceptions())
})

// defineExceptions are the settings.Define entries that stay out of the
// configuration registry. The census gate names each with its reason
// (gates/settingsdefineexceptions_test.go).
func defineExceptions() []settings.Definition {
	return slices.Concat(ai.Definitions(), capture.Definitions(), identity.Definitions(), privacy.Definitions())
}

// NewSettingsStore builds a store over the pool and the assembled catalog.
// Exported because it is genuinely part of the composition surface: the
// integration suites in compose/integration assemble the same stores the
// server does, and a second catalog built by hand there would drift from this
// one the moment a module declares a setting.
//
// The Store is a stateless view — pool plus the static registry — so building
// one per consumer costs nothing and avoids threading a shared instance
// through every job constructor.
func NewSettingsStore(pool *pgxpool.Pool) *settings.Store {
	return settings.New(pool, settingsRegistry())
}

// settingsRegistry is the assembled catalog, built once. The data reset needs
// it without a pool — it runs inside a transaction the caller already holds.
var settingsRegistry = sync.OnceValue(func() *settings.Registry {
	return settings.NewRegistry(settingsDefinitions()...)
})

// SettingSpec is the fitness gate's view of one registered setting: plain
// data, so the root-package gates can judge the catalog without importing
// platform (arch-lint grants root only contracts + compose — the same
// constraint NewTaskCensus is built for).
type SettingSpec struct {
	Key        string
	Object     string
	AuditVerb  string
	DefaultErr error
	HasFreeze  bool
	// Admits is the entry's own validator, so a gate can hold a bound some
	// other document states to the code that actually refuses past it.
	Admits func(json.RawMessage) error
}

// SettingsCatalogForTest flattens the assembled catalog for
// settingscatalog_test.go. Nothing in the product calls it.
func SettingsCatalogForTest() []SettingSpec {
	defs := settingsDefinitions()
	out := make([]SettingSpec, 0, len(defs))
	for _, d := range defs {
		_, err := d.DefaultJSON()
		out = append(out, SettingSpec{
			Key: d.Key(), Object: d.Object(), AuditVerb: d.AuditVerb(), DefaultErr: err,
			HasFreeze: d.HasFreezeProbe(), Admits: d.ValidateJSON,
		})
	}
	return out
}

// DeploymentConfigKeysForTest returns every dotted yaml path margince.yaml
// takes: the bootstrap struct's, and every boot setting's key. Derived rather
// than read off the example file, which omits whole sections and would let the
// surface-disjointness gate pass vacuously.
func DeploymentConfigKeysForTest() map[string]bool {
	keys := yamlPaths(reflect.TypeFor[deployconfig.Bootstrap](), "")
	for _, s := range allSettings() {
		if !s.Stored() {
			keys[s.Key()] = true
		}
	}
	return keys
}

// yamlPaths walks a config struct and returns every dotted yaml path in it,
// so the collision check tracks the schema rather than a hand-kept list.
func yamlPaths(t reflect.Type, prefix string) map[string]bool {
	// Unwrap pointers, and slices/arrays too: a config section can be a list
	// of structs (seeds.consent_purposes), and stopping at the slice would
	// leave its fields out of the collision set — a gate that is a subset of
	// the real config surface silently misses exactly the nested keys a
	// setting is most likely to shadow.
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil
	}
	out := map[string]bool{}
	for f := range t.Fields() {
		tag, _, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if tag == "" || tag == "-" {
			continue
		}
		path := tag
		if prefix != "" {
			path = prefix + "." + tag
		}
		out[path] = true
		maps.Copy(out, yamlPaths(f.Type, path))
	}
	return out
}

// DealsInstallation is the installation seam the deals module reads through.
// The wiring lives in installseam, which the integration harness can also
// reach. The owner hand-over here also tells the new owner, which needs the
// notifier in compose; the harness keeps installseam's hand-over without it.
func DealsInstallation() deals.Installation {
	inst := installseam.Deals()
	inst.FollowOwner = followDealOwnerAndTell
	return inst
}

// ContractTimezone is the installation timezone seam the contracts module reads
// its calendar "today" through — the SAME setting deals resolves, so a contract
// and a deal read one installation day.
//
// The UNGATED reader: deriving "today" is internal to a write the caller is
// already authorized for, and a contract writer holds `contract`, not
// `installation_settings` (identity.TimezoneAppliedTx says why).
func ContractTimezone() contracts.TimezoneFunc { return identity.TimezoneAppliedTx }
