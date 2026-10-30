// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a case read tells each reader. It says where a merged contact went and
// which pairs this reader may merge one by one. It names who owns a share of
// the case, and which members a large case shows.

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAResolvedCaseNamesNoSurvivorItsReaderCannotSee(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	source, survivor := members[0], members[1]
	if _, err := c.store.MergeContact(c.e.Admin(), ids.From[ids.ContactKind](source), ids.From[ids.ContactKind](survivor), nil); err != nil {
		t.Fatalf("merging: %v", err)
	}
	c.e.WsExec(t, `UPDATE contact SET visibility = 'owner' WHERE id = $1`, survivor)

	owner := c.read(c.rep(c.e.Rep2), t, source)
	if owner.Resolution == nil || ids.UUID(owner.Resolution.SurvivorId) != survivor {
		t.Fatalf("the survivor's owner reads resolution %+v, want the survivor", owner.Resolution)
	}
	stranger := c.read(c.rep(c.e.Rep3), t, source)
	if stranger.Status != "resolved" || stranger.Resolution != nil {
		t.Errorf("a reader who cannot see the survivor reads status %s and resolution %+v, want resolved and none",
			stranger.Status, stranger.Resolution)
	}
}

func TestAPairIsMergeableByTheOwnerOfEitherEnd(t *testing.T) {
	c := setupCases(t)
	shared := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	own := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	rep := c.rep(c.e.Rep1)

	half := c.read(rep, t, shared[0]).Edges[0]
	if half.CanDismiss || !half.CanMerge {
		t.Errorf("an owner of one end reads can_dismiss %v and can_merge %v, want false and true",
			half.CanDismiss, half.CanMerge)
	}
	if edge := c.read(c.rep(c.e.Rep3), t, shared[0]).Edges[0]; edge.CanMerge {
		t.Error("a rep who owns neither end may merge the pair")
	}
	c.setPolicy(t, true)
	if edge := c.read(rep, t, shared[0]).Edges[0]; !edge.CanDismiss || !edge.CanMerge {
		t.Errorf("under the policy an owner of one end reads can_dismiss %v and can_merge %v, want both",
			edge.CanDismiss, edge.CanMerge)
	}
	if edge := c.read(c.e.Admin(), t, shared[0]).Edges[0]; !edge.CanMerge {
		t.Error("an admin may not merge a pair they may write both ends of")
	}
	if edge := c.read(rep, t, own[0]).Edges[0]; !edge.CanMerge {
		t.Error("a rep may not merge a pair of their own two contacts")
	}
}

func TestAnAgentSeatOwnsNoShareOfACase(t *testing.T) {
	c := setupCases(t)
	agent := ids.NewV7()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`INSERT INTO app_user (id, email, display_name, is_agent) VALUES ($1, $2, 'Case Agent', true)`,
		agent, "agent-"+agent.String()+"@case.test"); err != nil {
		t.Fatal(err)
	}
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	c.e.WsExec(t, `UPDATE contact SET owner_id = $1 WHERE id = $2`, agent, members[2])
	c.setPolicy(t, true)
	rep := c.rep(c.e.Rep1)
	read := c.read(rep, t, members[0])
	for _, owner := range read.Owners {
		if ids.UUID(owner.UserId) == agent {
			t.Errorf("the case names the agent seat as an owner")
		}
	}
	if _, err := c.store.MergeContactCase(rep, members[0], caseDecision(read, members, members[0], c.e.Rep1)); err != nil {
		t.Fatalf("merging: %v", err)
	}
	if got := grantAccess(t, c.e, members[0], agent); got != "" {
		t.Errorf("the agent seat holds %q on the survivor, want no grant", got)
	}
}

func TestALargeCaseReadsAlikeFromEveryMember(t *testing.T) {
	c := setupCases(t)
	owners := make([]ids.UUID, 27)
	for i := range owners {
		owners[i] = c.e.Rep1
	}
	owners[26] = c.e.Rep2
	members := make([]ids.UUID, 0, len(owners))
	for _, owner := range owners {
		user := ids.From[ids.UserKind](owner)
		created, err := c.store.CreateContact(c.e.Admin(), contacts.CreateContactInput{
			FullName: "Mary Somerville", Source: "manual", OwnerID: &user,
			Phones: []contacts.ContactPhoneInput{{Phone: "+493055527000", PhoneType: "work", IsPrimary: true, Position: 1}},
		})
		if err != nil {
			t.Fatalf("creating a duplicate: %v", err)
		}
		members = append(members, ids.UUID(created.Id))
	}
	anchor := slices.MinFunc(members, compareUUIDs)
	for _, member := range members {
		got := c.read(c.e.Admin(), t, member)
		if ids.UUID(got.AnchorId) != anchor || !got.TooLarge || len(got.Members) != 25 {
			t.Errorf("from %s the case reads anchor %s, too_large %v and %d members, want %s, true and 25",
				member, got.AnchorId, got.TooLarge, len(got.Members), anchor)
		}
		if !slices.Contains(uuidsOf(memberIDs(got)), member) {
			t.Errorf("the case read from %s does not hold it", member)
		}
	}
	page, _, err := c.store.ListContactCases(c.rep(c.e.Rep2), contacts.ContactCaseListInput{Scope: "mine"})
	if err != nil || len(page) != 1 {
		t.Fatalf("Rep2's own cases = %d (%v), want the one case", len(page), err)
	}
	if ids.UUID(page[0].AnchorId) != anchor || !slices.Contains(uuidsOf(memberIDs(page[0])), members[26]) {
		t.Errorf("Rep2's listed case reads anchor %s without their member, want %s holding it", page[0].AnchorId, anchor)
	}
}
