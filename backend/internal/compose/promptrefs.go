// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// shownID is the record id a reply's ref names, or "" for a ref the request
// never showed, so a guessed or copied id never reaches a write.
func shownID(refs *promptref.Refs, ref string) string {
	if id, ok := refs.ID(strings.TrimSpace(ref)); ok {
		return id
	}
	return ""
}
