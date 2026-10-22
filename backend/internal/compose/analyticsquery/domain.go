// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"fmt"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// ValueDomain lists the values a filter on one field may name, because the
// population holds no row with any other. A filter outside it is refused: an
// answer of zero would read as a measured fact about rows that were never
// counted.
type ValueDomain struct {
	Values []string
	// Why is the reason the population stops there, in the asker's terms.
	Why string
}

// Admits refuses a value outside the domain. The zero domain admits anything.
//
//craft:ignore naked-any filter values are the decoded JSON plan, schemaless by design
func (d ValueDomain) Admits(field string, value any) error {
	if len(d.Values) == 0 {
		return nil
	}
	if text, ok := value.(string); ok && slices.Contains(d.Values, text) {
		return nil
	}
	return &RefusalError{
		Kind:    RefusalUnsupported,
		Message: fmt.Sprintf("%s cannot be %s here: %s", httperr.QuoteCaller(field), httperr.QuoteCaller(fmt.Sprint(value)), d.Why),
		Suggest: fmt.Sprintf("filter %s on one of: %s", httperr.QuoteCaller(field), strings.Join(d.Values, ", ")),
	}
}
