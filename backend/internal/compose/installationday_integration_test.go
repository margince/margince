// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// installationDay is the calendar day the at-risk and overdue scans judge at.
// A fixture dated from it lands on the scan's day in any machine zone.
func installationDay(t *testing.T, e *integration.Env, at time.Time) time.Time {
	t.Helper()
	zone, err := installationZone(e.Admin(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	return storekit.WorkspaceDay(at, zone)
}
