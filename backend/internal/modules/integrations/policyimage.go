// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

import (
	"cmp"
	"slices"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// poolPolicy is the part of a pool's budget a user sets. The reported balance
// shares the row but is not a setting, so a moved balance is not a change.
type poolPolicy struct {
	Pool              string
	MonthlyCeiling    *int
	PauseBelowBalance *int
}

// policyImage renders the settings a configuration save can change. Categories
// and pools are sorted because the request carries them as a map.
//
//craft:ignore naked-any the image is the schemaless column map storekit compares
func policyImage(c currentConfig) map[string]any {
	categories := slices.Clone(c.Categories)
	slices.Sort(categories)
	pools := make([]poolPolicy, 0, len(c.Budgets))
	for _, b := range c.Budgets {
		pools = append(pools, poolPolicy{Pool: b.Pool, MonthlyCeiling: b.MonthlyCeiling, PauseBelowBalance: b.PauseBelowBalance})
	}
	slices.SortFunc(pools, func(a, b poolPolicy) int { return cmp.Compare(a.Pool, b.Pool) })
	return map[string]any{
		auditKeyMode: c.Mode, auditKeyPreset: c.Preset, "categories": categories,
		auditKeyAutoCreate: c.AutomaticCreate, auditKeyAutoImport: c.AutomaticImport,
		"refresh_after_days": c.RefreshAfterDays, "daily_run_limit": c.DailyRunLimit,
		"budgets": pools,
	}
}

// configurationUnchanged answers whether a save leaves the policy as it stands.
// Such a save must not move the version or write an audit row.
func configurationUnchanged(current, merged currentConfig) bool {
	_, after := storekit.ChangedColumns(policyImage(current), policyImage(merged))
	return len(after) == 0
}
