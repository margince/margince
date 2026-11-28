// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package messengeraccounts_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type authorityMessenger struct {
	messaging.Client
	sends  int
	onRead func() error
}

func (*authorityMessenger) Identity(context.Context) (messaging.Identity, error) {
	return messaging.Identity{Workspace: "TEAM", Member: messaging.Member{ID: "REP", Active: true}}, nil
}

func (*authorityMessenger) Conversation(context.Context, string) (messaging.Conversation, error) {
	return messaging.Conversation{ID: "ROOM", Kind: "group", AudienceKnown: true, Members: []messaging.Member{{ID: "REP", Active: true}, {ID: "OTHER", Active: true}}}, nil
}

func (c *authorityMessenger) Messages(context.Context, string, string, string) (messaging.Page, error) {
	if c.onRead != nil {
		if err := c.onRead(); err != nil {
			return messaging.Page{}, err
		}
	}
	return messaging.Page{Messages: []messaging.Message{{ID: "PRIVATE", Text: "Confidential conversation"}}}, nil
}

func (c *authorityMessenger) Send(context.Context, messaging.Send) (messaging.Receipt, error) {
	c.sends++
	return messaging.Receipt{ID: "MESSAGE", Conversation: "ROOM"}, nil
}

func TestMessengerRechecksLiveAuthorityBeforeSendingWithAnOldSession(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	e.GrantRole(t, e.Rep1, "rep")
	ctx := integration.OnSeat(e.As(e.Rep1, nil, integration.AccountRepPerms), principal.SeatFull)
	company := e.SeedCompany(t, "Account", &e.Rep1)
	client := &authorityMessenger{}
	vault := keyvault.NewMemory()
	store := chatlinks.NewStore(e.DB(), nil, nil, nil).WithMessenger(func() keyvault.Vault { return vault }, func(context.Context, chatlinks.MemberGrant, []byte) (messaging.Client, error) { return client, nil }).WithMemberAuthority(messengeraccounts.MemberAuthority)
	grant, err := store.ConnectMember(ctx, chatlinks.MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "REP"}, []byte("synthetic-token"))
	if err != nil {
		t.Fatal(err)
	}
	room, err := client.Conversation(ctx, "ROOM")
	if err != nil {
		t.Fatal(err)
	}
	audience, err := messaging.AudienceKey(room)
	if err != nil {
		t.Fatal(err)
	}
	request := chatlinks.MessengerSend{RequestID: ids.NewV7(), GrantVersion: grant.Version, Conversation: "ROOM", Text: "Please check the wording.", AudienceKey: audience}
	first, err := store.SendMessenger(ctx, "company", company, "slack", request)
	if err != nil || first.State != "accepted" || client.sends != 1 {
		t.Fatalf("live sender: %+v %v sends=%d", first, err, client.sends)
	}
	admin := identity.Identity{UserID: ids.From[ids.UserKind](e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](e.WS), Roles: []string{"admin"}, SeatType: "full", Permissions: integration.AdminPerms}
	members := identity.NewServiceFor(e.DB()).WithMemberGrantWithdrawal(chatlinks.WithdrawMemberGrantsTx)
	identity.NewHandlers(members).WithCaptureCredentialReaper(func(ctx context.Context, user ids.UserID) {
		if err := store.ReapWithdrawnMemberCredentials(ctx, user); err != nil {
			t.Error(err)
		}
	})
	credentialRef := e.WsScalar(t, "SELECT credential_ref FROM chat_member_grant WHERE user_id=@user", pgx.StrictNamedArgs{"user": e.Rep1})
	client.onRead = func() error {
		return members.DeactivateUser(e.Admin(), admin, identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](e.Rep1)})
	}
	page, err := store.MessengerMessages(ctx, "company", company, "slack", "ROOM", "", "")
	if err == nil || len(page.Messages) != 0 {
		t.Fatalf("private result escaped after deactivation: %+v %v", page, err)
	}

	if _, _, err := store.OpenMember(ctx, "slack"); err == nil {
		t.Fatal("departed member reopened a private provider grant using an old session")
	}
	request.RequestID = ids.NewV7()
	if _, err := store.SendMessenger(ctx, "company", company, "slack", request); err == nil {
		t.Fatal("departed member sent using an old full-seat session")
	}
	if client.sends != 1 {
		t.Fatal("deactivated member reached the provider")
	}
	if _, err := store.ConnectMember(ctx, chatlinks.MemberGrant{Provider: "slack", Workspace: "TEAM", Member: "REP"}, []byte("synthetic-new-token")); err == nil {
		t.Fatal("departed member reconnected using an old session")
	}
	if _, err := vault.Get(ctx, ids.From[ids.WorkspaceKind](e.WS), keyvault.Ref(credentialRef)); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("departed member secret survived: %v", err)
	}
	assertReactivatedMemberNeedsNewGrant(ctx, t, e, members, admin, store)
}

func assertReactivatedMemberNeedsNewGrant(ctx context.Context, t *testing.T, e *integration.Env, members *identity.Service, admin identity.Identity, store *chatlinks.Store) {
	t.Helper()
	if err := members.ReactivateUser(e.Admin(), admin, ids.From[ids.UserKind](e.Rep1)); err != nil {
		t.Fatal(err)
	}
	reset, _, err := members.IssuePasswordLink(e.Admin(), admin, ids.From[ids.UserKind](e.Rep1))
	if err != nil {
		t.Fatal(err)
	}
	if err := members.RedeemPasswordReset(ctx, reset, "SyntheticUnique7!Credential"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.OpenMember(ctx, "slack"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("reactivated member restored an old private grant: %v", err)
	}
}
