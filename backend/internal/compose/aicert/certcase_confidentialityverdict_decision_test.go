// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
)

func TestTheConfidentialityCaseHasADecisionForm(t *testing.T) {
	assertEveryScenarioHasADecisionForm(t, ai.TaskCaptureConfidentialityVerdict)
}
