// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (f *invitationFixture) propose(ctx context.Context, t *testing.T, contact crmcontracts.Id, subject string, options ...slot) proposalLink {
	t.Helper()
	in := crmcontracts.MeetingProposalRequest{ContactId: contact, AttendeeEmail: f.request.AttendeeEmail, Subject: subject, DurationMinutes: 60}
	for _, option := range options {
		in.Options = append(in.Options, struct {
			End   time.Time `json:"end"`
			Start time.Time `json:"start"`
		}{End: option.End, Start: option.Start})
	}
	link, err := f.store.CreateProposal(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	return link
}

func (f *invitationFixture) seedContact(t *testing.T, owner ids.UUID) crmcontracts.Id {
	t.Helper()
	contact := ids.NewV7()
	args := schedulingArgs{}
	if _, err := f.env.owner.Exec(f.ctx, `INSERT INTO contact(id,full_name,source,captured_by,owner_id)VALUES(`+args.add(contact)+`,'Other guest','manual','human:test',`+args.add(owner)+`)`, args...); err != nil {
		t.Fatal(err)
	}
	return crmcontracts.Id(contact)
}

// actingAs is a colleague of the fixture's host with the same grants, plus
// the delete a withdrawal needs.
func (f *invitationFixture) actingAs(user ids.UUID) context.Context {
	return principal.WithActor(f.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true, Delete: true},
				"contact":  {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func TestTheProposalListShowsOnlyTheHostsLinksAGuestCanStillUse(t *testing.T) {
	f := newInvitationFixture(t)
	host := f.actingAs(f.env.rep)
	contact := f.request.ContactId

	expiring := f.propose(host, t, contact, "Expires first", slot{Start: monday(9), End: monday(10)})
	open := f.propose(host, t, contact, "Still open")
	used := f.propose(host, t, contact, "Already booked")
	withdrawn := f.propose(host, t, contact, "Withdrawn")
	newest := f.propose(host, t, contact, "Newest")
	f.propose(host, t, f.seedContact(t, f.env.rep), "Somebody else")
	f.propose(f.actingAs(f.env.other), t, contact, "A colleague's")

	if _, err := f.store.reserveAndQueueInvitation(host, ids.From[ids.UserKind](f.env.rep), f.request, invitationIntent{ProposalID: used.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ArchiveActivity(host, ids.From[ids.ActivityKind](withdrawn.ID), nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.store.ResolveProposalToken(host, strings.Split(withdrawn.URL, "proposal-")[1]); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a withdrawn proposal still opens for the guest: %v", err)
	}
	f.now = expiring.Expires.Add(time.Minute)

	listed, err := f.store.OpenProposals(host, ids.UUID(contact))
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, proposal := range listed {
		subjects = append(subjects, proposal.Subject)
	}
	if strings.Join(subjects, ",") != "Newest,Still open" {
		t.Fatalf("open proposals = %v, want the host's two live links newest first", subjects)
	}
	if listed[0].Url != newest.URL || listed[1].Url != open.URL || listed[1].Options == nil {
		t.Fatalf("listed links do not reopen the proposals: %+v", listed)
	}
}

func TestTheProposalListHidesAContactTheReaderCannotSee(t *testing.T) {
	f := newInvitationFixture(t)
	host := f.actingAs(f.env.rep)
	contact := f.seedContact(t, f.env.other)
	f.propose(host, t, contact, "Private talks")
	args := schedulingArgs{}
	if _, err := f.env.owner.Exec(f.ctx, `UPDATE contact SET visibility='owner' WHERE id=`+args.add(ids.UUID(contact)), args...); err != nil {
		t.Fatal(err)
	}

	listed, err := f.store.OpenProposals(host, ids.UUID(contact))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a contact made private to a colleague answered %+v, %v; want ErrNotFound", listed, err)
	}
}

func TestTheProposalListNeedsTheContactGrant(t *testing.T) {
	f := newInvitationFixture(t)
	f.propose(f.actingAs(f.env.rep), t, f.request.ContactId, "Before the grant was lost")
	withoutContacts := principal.WithActor(f.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + f.env.rep.String(), UserID: f.env.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true, Delete: true}},
			RowScope: principal.RowScopeAll,
		},
	})

	listed, err := f.store.OpenProposals(withoutContacts, ids.UUID(f.request.ContactId))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader without contact access listed %+v, %v; want ErrPermissionDenied", listed, err)
	}
}
