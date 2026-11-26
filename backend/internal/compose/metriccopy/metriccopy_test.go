// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package metriccopy

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestEveryMetricPhraseIsWrittenInEveryLanguage(t *testing.T) {
	langcopytest.Census(t, Names)
	langcopytest.Census(t, Definitions)
	langcopytest.Census(t, Attribution)
	langcopytest.Census(t, Incomplete)
}
