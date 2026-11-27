// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A chat message follows its contact as mail does. One contact has a mail and a
// Telegram Business chat message filed by the same member. Each change of the
// contact's sharing, through the real publish and patch paths, must
// change who can read both activities in the same way.

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// readersOf answers which seats can read one activity's content through the
// real visibility gate.
func readersOf(t *testing.T, e *integration.SearchEnv, activity ids.UUID) []ids.UUID {
	t.Helper()
	seats := map[ids.UUID][]ids.UUID{
		e.Rep1: {e.Team1}, e.Rep3: {e.Team2},
	}
	var readers []ids.UUID
	for seat, teams := range seats {
		ctx := principal.WithWorkspaceID(context.Background(), e.WS)
		ctx = principal.WithActor(ctx, principal.Principal{
			Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat, TeamIDs: teams,
			SeatType: principal.SeatFull,
			Permissions: principal.Permissions{
				RoleKeys: []string{"rep"},
				Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}},
				RowScope: principal.RowScopeTeam,
			},
		})
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return auth.EnsureActivityContentVisible(ctx, tx, activity)
		})
		if err == nil {
			readers = append(readers, seat)
		}
	}
	slices.SortFunc(readers, func(a, b ids.UUID) int { return strings.Compare(a.String(), b.String()) })
	return readers
}

func TestAChatMessageFollowsItsContactsSharingExactlyAsMailDoes(t *testing.T) {
	for _, seat := range []struct {
		name, posture string
		team          bool
	}{
		{"member seat shared", "shared", false},
		{"member seat classified", "classified", false},
		{"team seat shared", "shared", true},
	} {
		t.Run(seat.name, func(t *testing.T) { chatFollowsContactSharing(t, seat.posture, seat.team) })
	}
}

// chatSeatFor gives the chat account its seat under one posture. A team seat is
// read by a team of the mail's owner alone. Until the contact is shared, the
// seat and the owner's mail then have the same readers.
func chatSeatFor(t *testing.T, e *integration.SearchEnv, posture string, team bool) ids.UUID {
	t.Helper()
	var seat ids.UUID
	admin := principal.WithCorrelationID(e.Admin(), ids.NewV7())
	err := database.WithWorkspaceTx(admin, e.Pool, func(tx pgx.Tx) error {
		spec := capturemod.ChatSeat{Provider: "telegram_business", Account: "880001", Owner: e.Rep1, By: e.Rep1}
		if team {
			spec.Owner, spec.ReaderTeam = ids.Nil, ids.NewV7()
			if _, err := tx.Exec(context.Background(), `INSERT INTO team (id, name) VALUES ($1, 'Chat desk')`, spec.ReaderTeam); err != nil {
				return err
			}
			if _, err := tx.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, spec.ReaderTeam, e.Rep1); err != nil {
				return err
			}
		}
		var err error
		if seat, err = capturemod.EnsureChatSeatTx(admin, tx, spec); err != nil {
			return err
		}
		_, err = tx.Exec(context.Background(), `UPDATE capture_connection SET mail_posture = $2 WHERE id = $1`, seat, posture)
		return err
	})
	if err != nil {
		t.Fatalf("seating the chat account: %v", err)
	}
	return seat
}

// chatFixture is one member's mail with a contact and the chat account of that
// contact, under one seat posture. The contact is private to its owner.
type chatFixture struct {
	env     captureEnv
	e       *integration.SearchEnv
	mail    ids.UUID
	contact ids.ContactID
	binding connector.ChatBinding
	ctx     context.Context
}

// newChatFixture sets the mailbox posture, files the contact's first mail, binds
// the contact's chat identity and seats the chat account.
func newChatFixture(t *testing.T, posture string, team bool) chatFixture {
	t.Helper()
	env := newCaptureEnv(t)
	e := env.e
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE capture_connection SET mail_posture = $2 WHERE user_id = $1`, e.Rep1, posture)
		return err
	}); err != nil {
		t.Fatalf("setting the mailbox posture: %v", err)
	}
	const counterparty = "buyer@kunde.example"
	env.syncSent(t, map[string]bool{"parity-1@kunde.example": true},
		emailSaying(counterparty, "parity-1@kunde.example", "", "danke für das Angebot"))
	env.sync(t, email(counterparty, "Buyer", captureOwner, "parity-1r@kunde.example", "parity-1@kunde.example"))
	if got := contactVisibility(t, e, counterparty); got != "owner" {
		t.Fatalf("the contact starts %q, want owner", got)
	}
	mail := activityIDOf(t, e, "parity-1@kunde.example")
	var contactRaw ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT p.id FROM contact p JOIN contact_email pe ON pe.contact_id = p.id WHERE pe.email = $1`, counterparty).Scan(&contactRaw)
	}); err != nil {
		t.Fatalf("finding the contact: %v", err)
	}
	contact := ids.From[ids.ContactKind](contactRaw)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO contact_channel_identity (contact_id, provider, channel_user_id, username, source, captured_by)
			VALUES ($1, 'telegram', '990100', 'buyer', 'telegram', 'connector:telegram')`, contactRaw)
		return err
	}); err != nil {
		t.Fatalf("binding the chat identity: %v", err)
	}

	binding := connector.ChatBinding{
		Provider: "telegram", CapturedBy: "connector:telegram", KeyScope: "42:biz:880001", ThreadScope: "biz:880001",
		Ownership: connector.ChatOwnedByMember, OwnerUserID: e.Rep1, SeatID: chatSeatFor(t, e, posture, team),
	}
	ctx := connectorSeatCtx(e)
	if team {
		binding.Ownership, binding.OwnerUserID = connector.ChatOwnedByWorkspace, ids.Nil
		ctx = workspaceConnectorCtx(e)
	}
	return chatFixture{env: env, e: e, mail: mail, contact: contact, binding: binding, ctx: ctx}
}

// chatFollowsContactSharing runs the sequence with the mail and the chat seat
// under one posture. Under shared, publishing the contact opens both activities
// to a colleague. Under classified both stay held until a verdict judges them.
func chatFollowsContactSharing(t *testing.T, posture string, team bool) {
	fx := newChatFixture(t, posture, team)
	e, mail, contact, binding, ctx := fx.e, fx.mail, fx.contact, fx.binding, fx.ctx
	chat, err := capturemod.FileChatMessage(ctx,
		compose.NewCaptureSink(e.Pool, compose.CaptureConfig{}),
		binding,
		connector.ChatMessage{
			ConversationID: "990100", IdempotencyKey: "1", Direction: connector.DirectionInbound,
			Counterparty: connector.ChatParty{UserID: "990100", DisplayName: "Buyer"},
			Body:         "and the chat", OccurredAt: clocktest.Now(t),
		})
	if err != nil {
		t.Fatalf("filing the chat message: %v", err)
	}

	same := func(step string) []ids.UUID {
		t.Helper()
		fromMail, fromChat := readersOf(t, e, mail), readersOf(t, e, chat.ID)
		if !slices.Equal(fromMail, fromChat) {
			t.Fatalf("%s: mail is readable by %v and the chat message by %v, want the same seats", step, fromMail, fromChat)
		}
		return fromMail
	}

	if readers := same("a private contact"); !slices.Equal(readers, []ids.UUID{e.Rep1}) {
		t.Fatalf("a private contact's correspondence is readable by %v, want only its owner %v", readers, e.Rep1)
	}

	store := contacts.NewStore(e.DB()).WithSenderStamp(activities.StampProvenSendersTx)
	ownerCtx := func() context.Context { return ownerOf(e) }
	if err := store.PromoteOwnCapturedContact(ownerCtx(), contact); err != nil {
		t.Fatalf("publishing the contact: %v", err)
	}
	readers := same("a published contact")
	if posture == "shared" && len(readers) < 2 {
		t.Fatalf("a published contact's shared correspondence is readable by %v, want the colleague too", readers)
	}
	if posture == "classified" && len(readers) != 1 {
		t.Fatalf("a published contact's classified correspondence is readable by %v, want it held from the colleague until judged", readers)
	}

	private := "owner"
	if _, err := store.UpdateContact(ownerCtx(), contact, contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private again: %v", err)
	}
	same("a contact made private again")
}

// ownerOf is the member whose mail and chat the fixture holds, acting through
// the contact and activity writers.
func ownerOf(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{
				"contact": {Read: true, Update: true}, "activity": {Read: true, Update: true},
			},
		},
	})
}

// connectorSeatCtx is the connector principal the chat ingest runs as, acting
// for the seat that owns the connection.
func connectorSeatCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:telegram", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true, Read: true, Update: true},
				"contact":  {Create: true, Read: true, Update: true},
				"company":  {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// workspaceConnectorCtx is the connector principal the chat ingest runs as for a
// company account: it acts for no member.
func workspaceConnectorCtx(e *integration.SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:telegram",
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Create: true}, "contact": {Create: true}, "company": {Create: true, Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}
