// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package projects

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit/quickfindtest"
)

func TestTheProjectQuickFindReadsItsTrigramIndex(t *testing.T) {
	quickfindtest.AssertIndexed(t, "project", projectQuickFindExpr, "idx_project_name_trgm")
}
