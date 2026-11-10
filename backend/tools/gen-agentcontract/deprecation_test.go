// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"
)

func TestPublicDeprecationNoticesReachTheDownloadedContract(t *testing.T) {
	src := strings.Replace(fixture, "info: { title: Fixture, version: '1' }", "info: { title: Fixture, version: '1', x-api-deprecation-policy: {window_months: 3, patch_removals: false}}", 1)
	src = strings.Replace(src, "x-mcp-tool: { verb: list_records, tier: auto_execute, scope: read }", "x-mcp-tool: { verb: list_records, tier: auto_execute, scope: read }\n      deprecated: true\n      x-deprecation: {release: v1.0.0, announced_at: '2026-01-31T12:00:00Z'}", 1)
	out, _, _, err := agentContract([]byte(src), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"x-api-deprecation-policy:", "window_months: 3", "x-deprecation:", "2026-01-31T12:00:00Z"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("public notice missing %s", want)
		}
	}
	if strings.Contains(string(out), "x-mcp-tool:") || strings.Contains(string(out), "x-top-level:") {
		t.Fatal("internal annotations reached the client")
	}
}
