// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealverdict

import "testing"

func TestChangesNeedTheBuyersOwnCurrentWords(t *testing.T) {
	for _, c := range []struct {
		name, direction, kind, body, quote string
		want                               string
	}{
		{"buyer", "inbound", "email", "Our budget is 1,000 EUR.", "1,000 EUR", OutcomeApply},
		{"our price", "outbound", "email", "Our budget is 1,000 EUR.", "1,000 EUR", DroppedQuoteNotFound},
		{"note", "", "note", "Our budget is 1,000 EUR.", "1,000 EUR", DroppedQuoteNotFound},
		{"quoted price", "inbound", "email", "Thanks.\n> Our budget is 1,000 EUR.", "1,000 EUR", DroppedQuoteNotFound},
		{"forwarded price", "inbound", "email", "Thanks.\n-----Original Message-----\nOur budget is 1,000 EUR.", "1,000 EUR", DroppedQuoteNotFound},
		{"missing quote", "inbound", "email", "Our budget is 1,000 EUR.", "", DroppedQuoteNotFound},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := guardInput()
			in.Activities[0].Kind, in.Activities[0].Direction, in.Activities[0].Body = c.kind, c.direction, c.body
			probability := probabilityAction(20)
			probability.QuotedWords = c.quote
			decisions, err := Decide(in, guardState(), Payload{Actions: []Action{probability, amountAction(1000, "EUR", c.quote)}}, fixedRates{})
			if err != nil {
				t.Fatal(err)
			}
			if len(decisions) != 2 {
				t.Fatalf("decisions=%d, want both actions judged", len(decisions))
			}
			for _, decision := range decisions {
				if decision.Outcome != c.want {
					t.Errorf("%s: got %s, want %s", decision.Action.Kind, decision.Outcome, c.want)
				}
			}
		})
	}
}

func TestAnInboundCopyFromOurSideCannotSupplyTheBuyersQuote(t *testing.T) {
	in := guardInput()
	in.Activities[0].FromOurSide = true
	if buyerQuoteFound(in, []string{in.Activities[0].ID}, in.Activities[0].Body) {
		t.Fatal("our own inbound copy supplied a buyer quote")
	}
}
