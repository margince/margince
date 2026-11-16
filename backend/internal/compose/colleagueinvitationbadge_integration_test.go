// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The History badge and the Worklist read our own domains through the
// production wiring: the badge through the startup binding, the Worklist
// through the store's reader. A colleague's invitation captured before its
// domain was registered carries no seat stamp, and both say nothing is owed.
func TestTheBadgeAndTheWorklistReadOneOwnDomainListThroughTheProductionWiring(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	at := o.now.Add(-2 * time.Hour)
	colleague := o.captureAs(t, invitationMail("sam@colleagues.example", o.seat, "REQUEST", at), false)
	customer := o.captureAs(t, invitationMail("pat@customer.example", o.seat, "REQUEST", at), false)
	for _, message := range []ids.UUID{colleague, customer} {
		o.judgeAsRequest(t, message)
	}
	o.e.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ('colleagues.example', 'admin', true)`)

	worklist := activities.NewStore(o.e.DB()).WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(o.e.DB())})
	rows, err := worklist.WaitingReplies(o.reader(), o.now)
	if err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	onWorklist := map[ids.UUID]bool{}
	for _, row := range rows {
		onWorklist[row.ActivityID] = true
	}
	if o.badged(t, colleague) || onWorklist[colleague] {
		t.Errorf("a colleague's unstamped invitation: badge owed %v, Worklist %v; want neither", o.badged(t, colleague), onWorklist[colleague])
	}
	if !o.badged(t, customer) || !onWorklist[customer] {
		t.Errorf("a customer's invitation: badge owed %v, Worklist %v; want both, or the case above proves nothing",
			o.badged(t, customer), onWorklist[customer])
	}
}
