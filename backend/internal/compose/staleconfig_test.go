// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// The retired key must still load, or every operator file carrying it stops
// the boot, and it says once that it acts on nothing.
func TestARetiredModelPricingKeyStillLoadsAndWarns(t *testing.T) {
	var out bytes.Buffer
	WarnStaleRates(configtest.MustLoad(t, "rates:\n  model_pricing:\n    gemini: https://example.test/pricing\n", RatesModelPricing),
		slog.New(slog.NewTextHandler(&out, nil)))
	if n := strings.Count(out.String(), "level=WARN"); n != 1 {
		t.Fatalf("%d warnings, want one:\n%s", n, out.String())
	}
	if !strings.Contains(out.String(), "rates.model_pricing") || !strings.Contains(out.String(), "margince.yaml") {
		t.Errorf("warning %q names neither the retired key nor the file to edit", out.String())
	}

	out.Reset()
	WarnStaleRates(configtest.MustLoad(t, "version: 1\n", RatesModelPricing), slog.New(slog.NewTextHandler(&out, nil)))
	if out.Len() != 0 {
		t.Errorf("a file that says nothing stale was warned about: %s", out.String())
	}
}
