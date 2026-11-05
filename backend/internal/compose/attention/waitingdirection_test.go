// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import "testing"

func TestARequestDoesNotEstablishCommercialDirection(t *testing.T) {
	for _, nonBuyer := range []bool{false, true} {
		row := classifyWaiting(WaitingCustomer{Since: rankInstant, ConfirmedRequest: true, NonBuyer: nonBuyer}, rankInstant).item
		if nonBuyer {
			if row.Consequence != "none" || row.Level != levelRoutine || row.Because[0].Kind != "non_buyer_request" {
				t.Fatalf("non-customer request claimed buyer urgency: %+v", row)
			}
		} else if row.Consequence != "buyer_waits" || row.Level != levelWaiting {
			t.Fatalf("a short genuine request needs no reply history: %+v", row)
		}
	}
}
