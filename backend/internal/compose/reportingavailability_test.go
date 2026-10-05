// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"log/slog"
	"testing"
)

func TestRegistryIncludesReporting(t *testing.T) {
	registry := registryWithGate(InstallationDB(nil), nil, nil, SendPath{}, companyEnricher{}, nil, nil, nil, nil, slog.Default(), registryFeatures{})
	if _, found := registry.Spec("read_reporting"); !found {
		t.Fatal("reporting must be available without deployment options")
	}
}
