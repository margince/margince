// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importcells

import (
	"strings"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// IsMarginceID recognizes the version-7 format used for Margince record IDs.
func IsMarginceID(value string) bool {
	id, err := ids.Parse(value)
	return err == nil && id[6]>>4 == 7
}

// SamplesFit refuses a header suggestion unless its sampled cells fit the target.
func SamplesFit(p migration.Profile, header string, fits func(string) bool) bool {
	seen := false
	for _, col := range p.Columns {
		if col.Header != header {
			continue
		}
		for _, sample := range col.Samples {
			value := strings.TrimSpace(sample)
			if value == "" {
				continue
			}
			if !fits(value) {
				return false
			}
			seen = true
		}
	}
	return seen
}
