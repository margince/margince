// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The Company 360 state strip, its no-reply advice and each contact's
// engagement say whose move it is by the History badge's rules. The mail goes
// through the production parser and sink. A colleague's mail and an old
// invitation the calendar pass has not checked move nothing; plain mail does.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// accountOf creates a company employing a contact who writes from address.
func (o *owedEnv) accountOf(t *testing.T, name, address string) ids.CompanyID {
	t.Helper()
	company := ids.From[ids.CompanyKind](o.e.SeedCompany(t, name, nil))
	contact, primary := ids.From[ids.ContactKind](o.contact(t, "Pat at "+name, address)), true
	if _, err := o.e.Contacts.CreateRelationship(o.e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contact, CompanyID: &company, IsCurrentPrimary: &primary,
	}); err != nil {
		t.Fatalf("employing the contact at %s: %v", name, err)
	}
	return company
}

// shareMailbox makes a seat's captured mail workspace mail, so the reader may
// see a colleague's correspondence at all.
func (o *owedEnv) shareMailbox(t *testing.T, seat ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(context.Background(),
			`UPDATE capture_connection SET mail_posture = 'shared' WHERE user_id = $1`, seat)
		if err == nil && tag.RowsAffected() != 1 {
			return fmt.Errorf("%d mailboxes shared, want the seat's one", tag.RowsAffected())
		}
		return err
	}); err != nil {
		t.Fatalf("sharing the colleague's mailbox: %v", err)
	}
}

// whoseMove reads the account as the reader sees it: the strip's state, whether
// the no-reply advice fired, and the one contact's engagement.
func (o *owedEnv) whoseMove(t *testing.T, company ids.CompanyID) (string, bool, crmcontracts.ContactEngagement) {
	t.Helper()
	service := company360.NewService(o.e.Pool, o.e.Contacts, o.e.Deals, o.e.Projects,
		approvals.NewService(InstallationDB(o.e.Pool)), func() time.Time { return o.now })
	page, err := service.Assemble(o.reader(), company)
	if err != nil {
		t.Fatalf("assembling the account page: %v", err)
	}
	if page.StateStrip == nil || page.StateStrip.Engagement == nil {
		t.Fatal("the account page carries no engagement state")
	}
	advised := false
	if page.Suggestions != nil {
		for _, s := range *page.Suggestions {
			advised = advised || s.Kind == crmcontracts.Company360SuggestionKindNoReply
		}
	}
	list, err := service.ContactPage(o.reader(), company, company360.ContactListQuery{})
	if err != nil || len(list.Data) != 1 {
		t.Fatalf("the contact list: %v, %+v", err, list.Data)
	}
	return string(page.StateStrip.Engagement.State), advised, list.Data[0].Engagement
}

func TestCompany360ReadsWhoseMoveByTheBadgesRules(t *testing.T) {
	o := setupOwed(t)
	colleague := seatAddress(t, o.e, o.e.Rep2)
	o.shareMailbox(t, o.e.Rep2)
	fortnight := o.now.Add(-14 * 24 * time.Hour)

	colleagues := o.accountOf(t, "Colleague Wrote GmbH", "pat@colleague.example")
	o.captureFor(t, o.e.Rep2, colleague, owedMail{
		from: colleague, to: "pat@colleague.example", subject: "Our quote", at: fortnight,
		messageID: "colleague-" + ids.NewV7().String() + "@ws.example",
	})

	unchecked := o.accountOf(t, "Old Invitation GmbH", "pat@invite.example")
	invitation := o.captureAs(t, invitationMail("pat@invite.example", o.seat, "REQUEST", fortnight), false)
	o.asCapturedBeforeTheColumn(t, invitation, time.Hour)

	ours := o.accountOf(t, "We Wrote GmbH", "pat@ours.example")
	o.weWrite(t, owedMail{to: "pat@ours.example", subject: "The quote", at: fortnight})

	theirs := o.accountOf(t, "They Asked GmbH", "pat@theirs.example")
	o.judgeAsRequest(t, o.customerWrites(t, "pat@theirs.example", "Can we talk pricing?", fortnight))

	for _, c := range []struct {
		name    string
		company ids.CompanyID
		state   string
		advised bool
		// engagement is the contact's state. Empty asks only that the mail moved
		// it to neither waiting nor answered.
		engagement crmcontracts.ContactEngagement
	}{
		{"a colleague's mail", colleagues, "active", false, ""},
		{"an unchecked old invitation", unchecked, "active", false, crmcontracts.ContactEngagementUntried},
		{"the reader's plain mail", ours, "waiting_on_them", true, crmcontracts.ContactEngagementNoReply},
		{"their plain request", theirs, "waiting_on_us", false, crmcontracts.ContactEngagementWaiting},
	} {
		state, advised, engagement := o.whoseMove(t, c.company)
		if state != c.state {
			t.Errorf("%s: the strip reads %q, want %q", c.name, state, c.state)
		}
		if advised != c.advised {
			t.Errorf("%s: no-reply advice fired = %v, want %v", c.name, advised, c.advised)
		}
		switch {
		case c.engagement == "" && (engagement == crmcontracts.ContactEngagementWaiting ||
			engagement == crmcontracts.ContactEngagementAnswered):
			t.Errorf("%s: the contact reads %q, want neither waiting nor answered", c.name, engagement)
		case c.engagement != "" && engagement != c.engagement:
			t.Errorf("%s: the contact reads %q, want %q", c.name, engagement, c.engagement)
		}
	}
}

// The badge is judged from a mail's content, so a mail the reader may not read
// has no move for them. A colleague's private mail the classifier read as
// informing us still reads by its direction: the reader learns nothing of the
// verdict.
func TestCompany360DoesNotReadTheMoveOfAMailTheReaderMayNotRead(t *testing.T) {
	o := setupOwed(t)
	colleague := seatAddress(t, o.e, o.e.Rep2)
	company := o.accountOf(t, "Private Thread GmbH", "pat@private.example")
	private := o.captureFor(t, o.e.Rep2, colleague, owedMail{
		from: "pat@private.example", to: colleague, subject: "For your information",
		at:        o.now.Add(-14 * 24 * time.Hour),
		messageID: "private-" + ids.NewV7().String() + "@private.example",
	})
	judge := principal.WithActor(o.e.Admin(), o.classifier())
	if _, err := o.e.Activities.SetOwedVerdict(judge, private, activities.OwedVerdictInformsUs, "prompts-test", o.now); err != nil {
		t.Fatalf("judging the private mail: %v", err)
	}

	// Read by its verdict, the informing mail would leave the strip active. The
	// reader's strength does not count a mail they may not read at all.
	state, advised, engagement := o.whoseMove(t, company)
	if state != "waiting_on_us" || advised {
		t.Errorf("the strip reads %q with advice %v, want the direction's waiting_on_us and no advice", state, advised)
	}
	if engagement != crmcontracts.ContactEngagementUntried {
		t.Errorf("the contact reads %q, want untried: the reader's strength holds none of the private mail", engagement)
	}
}
