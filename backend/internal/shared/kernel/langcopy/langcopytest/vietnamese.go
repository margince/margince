// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// VietnameseText fails a Vietnamese sentence of member copy that breaks a
// mechanical rule of docs/reference/ui-copy-style-vi.md. Census applies it to
// every phrase; this is the call for a table that is not made of phrases.
func VietnameseText(t reporter, name, vietnamese string) {
	t.Helper()
	judge(t, member, textlang.Vietnamese, name, vietnamese, "")
}

// VietnameseText is the Vietnamese check in this register, which must name its reason.
func (r Register) VietnameseText(t reporter, name, vietnamese string) {
	t.Helper()
	judge(t, named(t, []Register{r}), textlang.Vietnamese, name, vietnamese, "")
}
