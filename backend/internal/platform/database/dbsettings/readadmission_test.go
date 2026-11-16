// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbsettings_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
)

func TestReadConcurrencyDefaultsAndOperatorOverrides(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want int
	}{
		{"", 3}, {"database:\n  read_concurrency: 5\n", 5},
	} {
		cfg, err := configtest.Load(t, tc.yaml, nil, nil, dbsettings.Settings()...)
		if err != nil {
			t.Fatal(err)
		}
		if got := config.Val(cfg, dbsettings.ReadConcurrency); got != tc.want {
			t.Fatalf("read concurrency = %d, want %d", got, tc.want)
		}
	}
	cfg := configtest.MustLoadEnv(t, dbsettings.Settings(), "MARGINCE_DATABASE_READ_CONCURRENCY=2")
	if got := config.Val(cfg, dbsettings.ReadConcurrency); got != 2 {
		t.Fatalf("environment read concurrency = %d, want 2", got)
	}
	if _, err := configtest.Load(t, "database:\n  read_concurrency: 0\n", nil, nil, dbsettings.Settings()...); err == nil {
		t.Fatal("accepted a zero allowance")
	}
}
