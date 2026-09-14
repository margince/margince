// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The address the SERVER picks is the address the shared rule picks.
//
// "The contact's email" is a decision, not a field: a contact carries several
// addresses, each with a position and a primary flag, some retired. That
// decision is now made once, in the read that builds the `emails` array — and
// the contacts list ORDERS BY the same expression, which is what a sort needs
// and what the browser could never provide.
//
// Two spellings of the rule already existed and are already held to each other
// (gates/frontendprimaryemail_test.go, over one case table). This is the arm
// that ties the SQL to them, over the cases that table calls awkward: a retired
// address first, a retired address that is ALSO marked primary, and a record
// with nothing live at all. Without it the SQL would be a third answer nothing
// compares.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedAddresses gives one contact their addresses in the order named, then
// retires the ones asked for — through the real writer, so the row under test
// is one the product really makes.
func seedAddresses(t *testing.T, e *Env, name string, in []contacts.ContactEmailInput, retire []string) ids.UUID {
	t.Helper()
	p, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", OwnerID: userIDPtr(&e.Rep1), Emails: in,
	})
	if err != nil {
		t.Fatalf("creating %q: %v", name, err)
	}
	for _, addr := range retire {
		e.WsExec(t, `UPDATE contact_email SET archived_at = now()
		             WHERE contact_id = $1 AND email = $2`, ids.UUID(p.Id), addr)
	}
	return ids.UUID(p.Id)
}

func TestTheServedAddressIsTheOneTheSharedRulePicks(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	for _, tc := range []struct {
		name    string
		emails  []contacts.ContactEmailInput
		retired []string
		want    string
	}{
		{
			name: "takes the one marked primary",
			emails: []contacts.ContactEmailInput{
				{Email: "old@buyer.test", EmailType: "work"},
				{Email: "anna@buyer.test", EmailType: "work", IsPrimary: true},
			},
			want: "anna@buyer.test",
		},
		{
			// An unmarked address is still reachable: the flag ranks, it does
			// not permit.
			name: "takes the first live one when nothing is marked",
			emails: []contacts.ContactEmailInput{
				{Email: "unmarked-a@buyer.test", EmailType: "work"},
				{Email: "unmarked-b@buyer.test", EmailType: "other"},
			},
			want: "unmarked-a@buyer.test",
		},
		{
			// The one answer that is actively wrong rather than merely
			// different: mail to a retired address bounces, or reaches a contact
			// who asked us to stop.
			name: "skips an archived address even when it is first",
			emails: []contacts.ContactEmailInput{
				{Email: "retired-first@buyer.test", EmailType: "work"},
				{Email: "live-after@buyer.test", EmailType: "work"},
			},
			retired: []string{"retired-first@buyer.test"},
			want:    "live-after@buyer.test",
		},
		{
			// Archived outranks primary: retirement is a decision about the
			// address itself.
			name: "skips an archived address even when it is marked primary",
			emails: []contacts.ContactEmailInput{
				{Email: "retired-marked@buyer.test", EmailType: "work", IsPrimary: true},
				{Email: "live-unmarked@buyer.test", EmailType: "work"},
			},
			retired: []string{"retired-marked@buyer.test"},
			want:    "live-unmarked@buyer.test",
		},
		{
			name: "answers nothing when every address is archived",
			emails: []contacts.ContactEmailInput{
				{Email: "all-gone@buyer.test", EmailType: "work"},
			},
			retired: []string{"all-gone@buyer.test"},
			want:    "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := seedAddresses(t, e, tc.name, tc.emails, tc.retired)
			got := servedContact(ctx, t, e, id)
			switch {
			case tc.want == "":
				if got.PrimaryEmail != nil {
					t.Fatalf("primary_email = %q, want none", string(*got.PrimaryEmail))
				}
			case got.PrimaryEmail == nil:
				t.Fatalf("primary_email is absent, want %q", tc.want)
			case string(*got.PrimaryEmail) != tc.want:
				t.Fatalf("primary_email = %q, want %q", string(*got.PrimaryEmail), tc.want)
			}
		})
	}
}

// servedContact reads one contact off the list — the surface whose cell prints
// the address and whose header orders by it.
func servedContact(ctx context.Context, t *testing.T, e *Env, id ids.UUID) crmcontracts.Contact {
	t.Helper()
	rows, _, err := e.Contacts.ListContacts(ctx, contacts.ListContactsInput{})
	if err != nil {
		t.Fatalf("listing contacts: %v", err)
	}
	for _, p := range rows {
		if ids.UUID(p.Id) == id {
			return p
		}
	}
	t.Fatalf("the contact just created is not on the list")
	return crmcontracts.Contact{}
}

// The contacts list orders by the address it prints — not by whichever address
// happens to sit first, which is what a sort over `contact_email` without the
// choosing rule would give.
func TestTheContactsListSortsByTheAddressItPrints(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, AccountRepPerms)

	// Each contact's FIRST address sorts the other way from their primary, so
	// a sort that took position alone returns the page reversed.
	zeta := seedAddresses(t, e, "Zeta Contact", []contacts.ContactEmailInput{
		{Email: "zzz@buyer.test", EmailType: "work"},
		{Email: "alma@buyer.test", EmailType: "work", IsPrimary: true},
	}, nil)
	alma := seedAddresses(t, e, "Alma Contact", []contacts.ContactEmailInput{
		{Email: "aaa@buyer.test", EmailType: "work"},
		{Email: "zeta@buyer.test", EmailType: "work", IsPrimary: true},
	}, nil)
	// No live address at all: nothing printed, nothing to order by.
	silent := seedAddresses(t, e, "Silent Contact", []contacts.ContactEmailInput{
		{Email: "gone@buyer.test", EmailType: "work"},
	}, []string{"gone@buyer.test"})

	spec := "primary_email"
	rows, _, err := e.Contacts.ListContacts(ctx, contacts.ListContactsInput{Sort: &spec})
	if err != nil {
		t.Fatalf("listing contacts by address: %v", err)
	}
	got := make([]ids.UUID, len(rows))
	for i, p := range rows {
		got[i] = ids.UUID(p.Id)
	}
	assertIDOrder(t, got, []ids.UUID{zeta, alma, silent},
		"address ascending — alma@ before zeta@, no address last")
}
