// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// A held draft is decided by the person it would go out as.
//
// Releasing one SENDS it, and the send takes its identity from the approving
// human — comms.stagingUser stamps the mailbox credential from the
// authenticated principal, and the From name and signature come from that same
// actor. So a colleague approving a rep's draft does not authorise the rep's
// message; they send their own, into a customer thread they were never part of,
// under their own name.
//
// The decision-grant map gives this kind the same grants as send_email, which
// four of five seeded roles hold and which admin and management hold over every
// row. Without the self-only clause below, that is who could send as somebody
// else.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sendGrantHolder is a human holding exactly what deciding a held draft
// requires: activity.CREATE, the send_email grant. The point of the test is
// that holding it is NOT enough.
func sendGrantHolder(user ids.UUID) principal.Principal {
	return principal.Principal{
		UserID: user,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{
			objectActivity: {Create: true, Read: true, Update: true, Delete: true},
		}},
	}
}

func TestAHeldDraftIsDecidedByTheRepItSendsAs(t *testing.T) {
	rep := ids.New[ids.UserKind]().UUID
	staged := row{Kind: kindHeldDraft, OnBehalfOf: ptr(ids.From[ids.UserKind](rep))}

	// A manager with every activity grant there is, who is not the rep.
	manager := sendGrantHolder(ids.New[ids.UserKind]().UUID)
	byManager, err := decidable(context.Background(), nil, manager, staged)
	if err != nil {
		t.Fatal(err)
	}
	if byManager {
		t.Error("a colleague holding activity.create can release another rep's held draft. Approving " +
			"SENDS it from the approver's own mailbox, under their name and signature — so this is not " +
			"one person authorising another's message, it is a stranger answering a live customer thread")
	}

	byRep, err := decidable(context.Background(), nil, sendGrantHolder(rep), staged)
	if err != nil {
		t.Fatal(err)
	}
	if !byRep {
		t.Error("the rep the draft was written for cannot release their own message")
	}
}

// A held draft recording nobody is decidable by NOBODY.
//
// This is the clause failing closed, and it is load-bearing rather than
// theoretical: every held draft staged before the owner was carried into
// staging has a null on_behalf_of. Reading "no owner recorded" as "anyone may
// send it" would restore the exact behaviour this narrowing removes, and would
// do it silently on precisely the rows nobody checked.
func TestAHeldDraftWithNoRepRecordedIsDecidedByNobody(t *testing.T) {
	staged := row{Kind: kindHeldDraft}

	for who, p := range map[string]principal.Principal{
		"an admin":    sendGrantHolder(ids.New[ids.UserKind]().UUID),
		"another rep": sendGrantHolder(ids.New[ids.UserKind]().UUID),
		"a principal with no user at all": {Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{objectActivity: {Create: true}},
		}},
	} {
		ok, err := decidable(context.Background(), nil, p, staged)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			t.Errorf("%s can release a held draft that records no rep. A message with nobody recorded is "+
				"one nobody may send, not one everybody may", who)
		}
	}
}
