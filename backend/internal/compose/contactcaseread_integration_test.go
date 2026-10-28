// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a case read tells each reader: where a merged contact went, and which
// pairs this reader may merge one by one.

import (
	"testing"

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

func TestAPairIsMergeableOnlyWithWriteAccessToBothEnds(t *testing.T) {
	c := setupCases(t)
	shared := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	own := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	c.setPolicy(t, true)
	rep := c.rep(c.e.Rep1)

	delegated := c.read(rep, t, shared[0]).Edges[0]
	if !delegated.CanDismiss || delegated.CanMerge {
		t.Errorf("an owner of one end under the policy reads can_dismiss %v and can_merge %v, want true and false",
			delegated.CanDismiss, delegated.CanMerge)
	}
	if edge := c.read(c.e.Admin(), t, shared[0]).Edges[0]; !edge.CanMerge {
		t.Error("an admin may not merge a pair they may write both ends of")
	}
	if edge := c.read(rep, t, own[0]).Edges[0]; !edge.CanMerge {
		t.Error("a rep may not merge a pair of their own two contacts")
	}
}
