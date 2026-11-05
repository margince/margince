// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestCaptureExplanationCoversEveryLanguage(t *testing.T) {
	langcopytest.Census(t, captureWords)
}
