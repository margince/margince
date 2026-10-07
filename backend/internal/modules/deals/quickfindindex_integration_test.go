// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit/quickfindtest"
)

func TestTheDealQuickFindReadsItsTrigramIndex(t *testing.T) {
	quickfindtest.AssertIndexed(t, "deal", dealNameColumn, "idx_deal_name_trgm")
}
