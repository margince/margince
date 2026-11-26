// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
)

// MachineTitle is a job title from a card or a provider, cut to what
// contact.title holds. Nobody can shorten such a title before it is written,
// so a longer one is cut instead of failing the whole import.
func MachineTitle(raw string) string {
	return strings.TrimSpace(textbounds.Fit("public.contact.title", strings.TrimSpace(raw)))
}
