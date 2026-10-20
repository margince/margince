// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package programsettings_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/programsettings"
)

// A deployment sets every boot setting from its container's environment, and
// an operator by hand from the command line, so each has both names.
func TestEveryProgramSettingHasAFlagAndAnEnvironmentName(t *testing.T) {
	t.Parallel()
	for _, s := range programsettings.Settings() {
		if s.Flag() == "" || s.Env() == "" {
			t.Errorf("%s: flag %q, env %q; a boot setting needs both", s.Key(), s.Flag(), s.Env())
		}
	}
}
