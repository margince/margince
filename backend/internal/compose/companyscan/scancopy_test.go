// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companyscan

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestScanFailureCopyCoversSupportedLanguages(t *testing.T) {
	langcopytest.Census(t, scanWords)
}
