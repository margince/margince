// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type identityClient struct {
	messaging.Client
	identity messaging.Identity
}

func (c identityClient) Identity(context.Context) (messaging.Identity, error) { return c.identity, nil }

func TestParticipantCredentialsArePrivateVerifiedAndWithdrawnOnDisconnect(t *testing.T) {
	e := setupLinks(t)
	vault := keyvault.NewMemory()
	factory := func(_ context.Context, _ MemberGrant, secret []byte) (messaging.Client, error) {
		member := "MEMBER1"
		if string(secret) == "second-member-token" {
			member = "MEMBER2"
		}
		if string(secret) == "rejected-token" {
			return nil, messaging.ErrUnauthorized
		}
		return identityClient{identity: messaging.Identity{Workspace: "WORKSPACE", Member: messaging.Member{ID: member, Name: member, Active: true}}}, nil
	}
	e.store.WithMessenger(func() keyvault.Vault { return vault }, factory)
	ctx := e.seat(e.rep)
	grant, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "MEMBER1"}, []byte("first-token"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ConnectMember(e.seat(e.other), MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "MEMBER1"}, []byte("first-token")); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("one provider account was claimed by another CRM member: %v", err)
	}
	oldRef := grant.credential
	if oldRef == "" || oldRef == "first-token" {
		t.Fatal("credential was not sealed")
	}
	if _, err := e.store.MemberGrant(e.seat(e.other), "slack"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("another rep saw the grant: %v", err)
	}
	if _, _, err := e.store.OpenMember(ctx, "slack"); err != nil {
		t.Fatal(err)
	}
	rotated, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "MEMBER1"}, []byte("rotated-token"))
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != grant.ID || rotated.Version <= grant.Version {
		t.Fatalf("rotation lost identity/version: %+v", rotated)
	}
	if _, err := vault.Get(ctx, ids.From[ids.WorkspaceKind](e.ws), keyvault.Ref(oldRef)); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("replaced credential survived: %v", err)
	}
	if _, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "MEMBER1"}, []byte("second-member-token")); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("identity mismatch accepted: %v", err)
	}
	if err := e.store.DisconnectMember(ctx, "slack"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.store.OpenMember(ctx, "slack"); !errors.Is(err, messaging.ErrUnauthorized) {
		t.Fatalf("disconnected grant opened: %v", err)
	}
	if _, err := vault.Get(ctx, ids.From[ids.WorkspaceKind](e.ws), keyvault.Ref(rotated.credential)); !errors.Is(err, keyvault.ErrNotFound) {
		t.Fatalf("disconnected secret survived: %v", err)
	}
	if _, err := e.store.MemberGrant(e.system(), "slack"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("system actor read private grant: %v", err)
	}
}
