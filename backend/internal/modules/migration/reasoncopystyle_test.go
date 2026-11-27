// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"maps"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryReasonSentenceFollowsTheStylePages(t *testing.T) {
	for _, code := range slices.Sorted(maps.Keys(reasonCatalog)) {
		text := reasonCatalog[code].text
		langcopytest.EnglishText(t, string(code), text[textlang.English])
		langcopytest.VietnameseText(t, string(code), text[textlang.Vietnamese])
	}
}
