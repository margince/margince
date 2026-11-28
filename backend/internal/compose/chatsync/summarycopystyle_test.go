// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestTheNoteHeadingsFollowTheStylePages(t *testing.T) {
	for _, heading := range noteHeadings[textlang.English] {
		langcopytest.EnglishText(t, "noteHeadings", heading)
	}
	for _, heading := range noteHeadings[textlang.Vietnamese] {
		langcopytest.VietnameseText(t, "noteHeadings", heading)
	}
}
