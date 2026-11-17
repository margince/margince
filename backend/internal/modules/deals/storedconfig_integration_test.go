// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
)

// Legal review switched off at boot releases the gate over a saved row that
// keeps it on.
func TestABootSetLegalReviewOffReleasesTheGate(t *testing.T) {
	t.Parallel()
	e := setupReviewEnv(t)
	dealID, _, toID := gatedDeal(t, e)
	boot := config.WithBound(e.asRequester(), configtest.StoredOver(t, e.pool, ConfigSettings(), "MARGINCE_DEALS_LEGAL_REVIEW_ENABLED=false"))

	if _, err := e.store.AdvanceDeal(boot, dealID, AdvanceDealInput{ToStageID: toID}); err != nil {
		t.Fatalf("the gate held a deal with legal review off at boot: %v", err)
	}
}
