// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The address trigger stamps a company's stale geocode status as a literal,
// and this file holds that literal to contacts.GeocodeStale. enumsync_test.go
// holds contacts.GeocodeStatuses() to the column's CHECK.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheAddressTriggerStampsTheGeocodeStaleStatus(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(filepath.Join(repoRoot, headCatalogPath))
	if err != nil {
		t.Fatalf("reading %s: %v", headCatalogPath, err)
	}
	if stamp := "SET status = '" + contacts.GeocodeStale + "'"; !strings.Contains(string(raw), stamp) {
		t.Errorf("the address trigger no longer stamps %q, so it and contacts.GeocodeStale have drifted", stamp)
	}
}
