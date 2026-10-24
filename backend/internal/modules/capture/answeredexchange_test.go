// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"strings"
	"testing"
)

// Both questions about an exchange are built from answeredExchange, and
// neither spells the exchange a second time beside it.
func TestBothExchangeQuestionsAreBuiltFromOnePredicate(t *testing.T) {
	for name, built := range map[string]struct{ sql, alias string }{
		"wroteBack":       {wroteBack(), "inbound"},
		"addressAnswered": {addressAnswered(), "reply"},
	} {
		exchange := answeredExchange(built.alias, "$1")
		if !strings.Contains(built.sql, exchange) {
			t.Errorf("%s does not contain the shared exchange predicate", name)
		}
		rest := strings.Replace(built.sql, exchange, "", 1)
		for _, clause := range []string{"bulk_mail_attested", "counterparty_outbound_attested", "direction = 'inbound'"} {
			if strings.Contains(rest, clause) {
				t.Errorf("%s spells %q outside the shared predicate", name, clause)
			}
		}
	}
}
