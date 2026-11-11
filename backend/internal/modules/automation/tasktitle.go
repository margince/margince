// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"unicode/utf8"
)

const maxTaskTitleCharacters = 200

func validTaskTitle(title string) bool {
	return values.HasVisibleText(title) && utf8.RuneCountInString(title) <= maxTaskTitleCharacters
}
