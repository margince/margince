// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestOwnershipTransferWordsCoverEveryLanguage(t *testing.T) {
	langcopytest.Census(t, ownershipTransferWords)
}
