// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

// A deployment sets every boot setting from its container's environment, and
// an operator by hand from the command line, so each has both names.
func TestEveryAPIBootSettingHasAFlagAndAnEnvironmentName(t *testing.T) {
	t.Parallel()
	for _, s := range APISettings() {
		if s.Flag() == "" || s.Env() == "" {
			t.Errorf("%s: flag %q, env %q; a boot setting needs both", s.Key(), s.Flag(), s.Env())
		}
	}
}

// A value the environment holds but its setting cannot read is reported with
// the other faults of the same boot, not instead of them.
func TestAnUnreadableEnvironmentValueJoinsTheBootFaults(t *testing.T) {
	t.Parallel()
	_, _, err := loadAPIConfig(nil, []string{"MARGINCE_INLINE_RELAY=maybe"})
	if err == nil {
		t.Fatal("loading answered no error, want both faults")
	}
	for _, want := range []string{"MARGINCE_DSN", "MARGINCE_INLINE_RELAY"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error does not mention %s:\n%s", want, err)
		}
	}
}
