// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The History badge and the Worklist read our own domains through the
// production wiring. The badge reads the startup binding, the Worklist the
// store's reader. A colleague's invitation captured before its
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

// The company page derives a contact's engagement from whose move a mail is,
// for a reader without the activity grant too. The own-domain list is
// workspace setting, so the colleague test reads it whoever asks. That reader
// is not told a colleague's invitation waits on us.
func TestWhoseMoveReadsOurDomainsForAReaderWithoutTheActivityGrant(t *testing.T) {
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

	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, "activity")
	reader := o.e.As(o.owner, nil, perms)
	var moves map[ids.UUID]crmcontracts.EmailSummaryMove
	if err := database.WithWorkspaceTx(reader, o.e.Pool, func(tx pgx.Tx) error {
		var err error
		moves, err = activities.MailMovesFor(reader, tx, []ids.UUID{colleague, customer}, o.now)
		return err
	}); err != nil {
		t.Fatalf("reading whose move each mail is without the activity grant: %v", err)
	}
	if moves[customer] != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Fatalf("a customer's invitation reads %q, want %q, or the case below proves nothing",
			moves[customer], crmcontracts.EmailSummaryMoveNeedsReply)
	}
	if moves[colleague] == crmcontracts.EmailSummaryMoveNeedsReply {
		t.Errorf("a colleague's unstamped invitation reads %q for a reader without the activity grant", moves[colleague])
	}
}
