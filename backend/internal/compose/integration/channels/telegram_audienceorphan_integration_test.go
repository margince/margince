// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package channels

// Narrowing a captured channel message must not leave it readable by nobody.
//
// A Telegram message satisfies exactly ONE arm of the activity audience gate:
// `audience = 'workspace'`. Its captured_by is a bare `connector:telegram` with
// no trailing uuid, no mailbox imported it, and the sink stamps no our-side
// participant because a bot binding has no owner. So a write that turns the
// first arm off turns every arm off — and the row cannot be widened back,
// because widening needs the content visibility the write just destroyed.
//
// The whole leg runs: the real poll, the real sink, the real audience writer.
// A test that inserted an activity row by hand would prove nothing about the
// arms, because the arms are about what capture actually wrote.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// audienceWriterCtx binds the seat that reaches PATCH /activities/{id}/audience
// on the shipped path: a human on a full seat holding activity.update, on the
// widest row scope, so nothing about ROW authority is what the assertions
// below are measuring.
//
// It is the SEEDED admin rather than a minted id, because `selected` names a
// real app_user row — and because the connecting admin holding no reader arm on
// a message their own bot captured is the sharper version of the claim: a
// channel connection's connected_by is audit-only and confers nothing.
func (c *telegramEnv) audienceWriterCtx(t *testing.T) context.Context {
	t.Helper()
	user, err := ids.Parse(c.admin)
	if err != nil {
		t.Fatalf("parsing the admin id: %v", err)
	}
	ctx := principal.WithWorkspaceID(context.Background(), c.workspaceID(t))
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + c.admin, UserID: user,
		SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Update: true},
				"person":   {Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// captureOneChannelMessage runs the whole ingress leg and answers the activity
// it produced, as the typed id the audience writer takes.
func captureOneChannelMessage(t *testing.T, c *telegramEnv, u telegramUpdate) ids.ActivityID {
	t.Helper()
	c.ingestOne(t, u, compose.JobRunnerConfig{})
	activityID, _ := c.capturedMessage(t, u)
	id, err := ids.ParseAs[ids.ActivityKind](activityID)
	if err != nil {
		t.Fatalf("parsing the captured activity id: %v", err)
	}
	return id
}

// TestNarrowingAChannelMessageToParticipantsIsRefused is the invariant: an
// audience write may not leave a message with no human reader.
//
// `participants` is the case that does it. The refusal is the fix rather than
// hiding the control in the browser, because the browser hiding a button stops
// no API client.
func TestNarrowingAChannelMessageToParticipantsIsRefused(t *testing.T) {
	c := setupTelegramConnected(t)
	id := captureOneChannelMessage(t, c,
		telegramUpdate{updateID: 7301, messageID: 41, senderID: 770301, username: "orphan", firstName: "Ann", text: "Do you still ship to Vienna?"})

	store := activities.NewStore(c.DB())
	writer := c.audienceWriterCtx(t)

	if _, err := store.SetAudience(writer, id,
		activities.SetAudienceInput{Audience: "participants"}); err == nil {
		t.Fatal("narrowing a captured channel message to participants was accepted — " +
			"the row now satisfies no arm of the audience gate and no human can read it or widen it back")
	}

	// The refusal has to be the WHOLE story: a partial write that narrowed the
	// row and then failed would orphan it exactly as the accepted write did.
	if _, err := store.GetActivityContent(writer, id, storekit.LiveOnly); err != nil {
		t.Fatalf("the message is unreadable after the refused write: %v — "+
			"the refusal committed part of the narrowing it exists to prevent", err)
	}
}

// TestNarrowingAChannelMessageToSelectedStillWorks holds the refusal to its
// reason rather than to the kind.
//
// `selected` naming somebody is survivable: the membership row IS an arm, so
// the message keeps a reader and can be widened back by them. A refusal that
// blocked this too would be a ban on narrowing channel messages, which is a
// different rule with a different justification — and one nobody has asked for.
func TestNarrowingAChannelMessageToSelectedStillWorks(t *testing.T) {
	c := setupTelegramConnected(t)
	id := captureOneChannelMessage(t, c,
		telegramUpdate{updateID: 7302, messageID: 42, senderID: 770302, username: "selected", firstName: "Bo", text: "Sending the PO now."})

	store := activities.NewStore(c.DB())
	writer := c.audienceWriterCtx(t)
	me, ok := principal.Actor(writer)
	if !ok {
		t.Fatal("the writer context carries no actor")
	}

	if _, err := store.SetAudience(writer, id, activities.SetAudienceInput{
		Audience: "selected",
		Members:  []activities.AudienceMember{{SubjectType: "user", SubjectID: me.UserID}},
	}); err != nil {
		t.Fatalf("narrowing a captured channel message to a named reader was refused: %v — "+
			"the rule is about leaving no reader, not about the kind", err)
	}

	if _, err := store.GetActivityContent(writer, id, storekit.LiveOnly); err != nil {
		t.Fatalf("the named reader cannot read the message they were named on: %v", err)
	}
}
