// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import "testing"

func TestRetiredReportingConfigurationStillLoads(t *testing.T) {
	for _, config := range []string{"version: 1\n", "version: 1\nanalytics:\n  performance_enabled: false\n", "version: 1\nanalytics:\n  performance_enabled: true\n"} {
		if _, err := Parse([]byte(config)); err != nil {
			t.Fatalf("existing configuration must still load: %v", err)
		}
	}
}
