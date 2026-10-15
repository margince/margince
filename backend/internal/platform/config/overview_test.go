// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"strings"
	"testing"
)

func TestTheOverviewAndTraceNameASecretsSourceButNotItsValue(t *testing.T) {
	cfg, err := load(t, fixture{env: []string{"MARGINCE_DSN=postgres://secret", "MARGINCE_LOG_LEVEL=debug"}}, level, dsn)
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range cfg.Entries() {
		switch e.Key {
		case "dsn":
			seen++
			if e.Value != nil || e.Origin.Name != "MARGINCE_DSN" || !e.Secret {
				t.Fatalf("dsn entry %+v", e)
			}
		case "log.level":
			seen++
			if e.Value == nil || *e.Value != "debug" {
				t.Fatalf("log.level entry %+v", e)
			}
		}
	}
	if seen != 2 {
		t.Fatalf("overview lists %d of the 2 settings", seen)
	}
	trace := strings.Join(cfg.TraceLines(), "\n")
	if strings.Contains(trace, "postgres://secret") || !strings.Contains(trace, "dsn = <secret> (env MARGINCE_DSN)") ||
		!strings.Contains(trace, "log.level = debug (env MARGINCE_LOG_LEVEL)") {
		t.Fatalf("trace:\n%s", trace)
	}
}
