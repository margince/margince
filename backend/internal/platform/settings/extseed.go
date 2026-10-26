// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/pkg/extension"
)

// ExtensionSeed is what one unit's seed did: the keys it stored and the keys
// that already held a value and were left alone.
type ExtensionSeed struct {
	Unit   extension.Name
	Stored []string
	Kept   []string
}

// SeedExtensions stores each composed unit's declared settings. A key that has
// a row is kept, so a second run changes nothing and an operator's later edit
// survives it. The caller's transaction makes a bad declaration leave the table
// as it was.
//
// It takes no RBAC gate, for Seed's reason: the operator's process runs it with
// the owner's DSN, and no signed-in member is involved.
func SeedExtensions(ctx context.Context, tx pgx.Tx, exts []extension.Extension) ([]ExtensionSeed, error) {
	var reports []ExtensionSeed
	for _, e := range exts {
		if e.Settings == nil {
			continue
		}
		values, err := extension.SettingsFrom(e.Settings)
		if err != nil {
			return nil, fmt.Errorf("settings: extension %q: %w", e.Name, err)
		}
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		report := ExtensionSeed{Unit: e.Name}
		for _, key := range keys {
			stored, err := e.Name.SettingKey(key)
			if err != nil {
				return nil, fmt.Errorf("settings: extension %q: %w", e.Name, err)
			}
			tag, err := tx.Exec(ctx,
				`INSERT INTO setting (key, value) VALUES ($1, $2) ON CONFLICT (key) DO NOTHING`,
				stored, []byte(values[key]))
			if err != nil {
				return nil, fmt.Errorf("settings: seeding %s: %w", stored, err)
			}
			if tag.RowsAffected() == 1 {
				report.Stored = append(report.Stored, key)
			} else {
				report.Kept = append(report.Kept, key)
			}
		}
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].Unit < reports[j].Unit })
	return reports, nil
}
