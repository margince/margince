// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import "github.com/margince/margince/backend/internal/shared/kernel/textlang"

// EnglishText fails an English sentence of member copy that breaks a rule of
// docs/reference/ui-copy-style.md. Census applies it to every phrase; this is
// the call for a table that is not made of phrases.
func EnglishText(t reporter, name, english string) {
	t.Helper()
	judge(t, member, textlang.English, name, english, english)
}

// EnglishText is the English check in this register, which must name its reason.
func (r Register) EnglishText(t reporter, name, english string) {
	t.Helper()
	judge(t, named(t, []Register{r}), textlang.English, name, english, english)
}
