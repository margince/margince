// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/programsettings"
)

func TestProgramSettingsExposeOnlyTheirAllowedSources(t *testing.T) {
	t.Parallel()
	for _, s := range programsettings.Settings() {
		if s.Key() == programsettings.CoreFeaturesDisabled.Key() {
			if s.Flag() != "" || s.Env() != "" || s.Stored() {
				t.Errorf("%s must be set only in a config file", s.Key())
			}
			continue
		}
		if s.Flag() == "" || s.Env() == "" {
			t.Errorf("%s: flag %q, env %q; a boot setting needs both", s.Key(), s.Flag(), s.Env())
		}
	}
}
