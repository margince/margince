// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package chatlinks

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/teamchat"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func setupSlackMemberLinks(t *testing.T) *linkEnv {
	t.Helper()
	e := setupLinks(t)
	e.exec(t, `UPDATE channel_connection SET provider=$1,address=NULL`, ProviderSlack)
	return e
}

func providerAccounts(e *linkEnv) {
	vault := keyvault.NewMemory()
	e.store.WithMessenger(func() keyvault.Vault { return vault }, func(_ context.Context, grant MemberGrant, _ []byte) (messaging.Client, error) {
		return identityClient{identity: messaging.Identity{Workspace: grant.Workspace, Member: messaging.Member{ID: grant.Member, Name: grant.Member, Active: true}}}, nil
	})
}

func TestSharedServicePreventsPersonalConsentForAnotherProvider(t *testing.T) {
	e := setupLinks(t)
	providerAccounts(e)
	_, err := e.store.ConnectMember(e.seat(e.rep), MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "REP"}, []byte("fixture-token"))
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("second service connected: %v", err)
	}
	var grants int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM chat_member_grant`).Scan(&grants); err != nil || grants != 0 {
		t.Fatalf("rejected consent persisted: %d %v", grants, err)
	}
}

func TestConcurrentPersonalConnectionsSelectOnlyOneWorkspaceService(t *testing.T) {
	e := setupLinks(t)
	e.exec(t, `DELETE FROM channel_connection`)
	providerAccounts(e)
	start := make(chan struct{})
	outcomes := make(chan error, 2)
	for _, provider := range []string{"slack", "teams"} {
		go func(provider string) {
			<-start
			_, err := e.store.ConnectMember(e.seat(e.rep), MemberGrant{Provider: provider, Workspace: "WORKSPACE", Member: "REP"}, []byte("fixture-token"))
			outcomes <- err
		}(provider)
	}
	close(start)
	connected, conflicts := 0, 0
	for range 2 {
		err := <-outcomes
		switch {
		case err == nil:
			connected++
		case errors.Is(err, apperrors.ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected result: %v", err)
		}
	}
	if connected != 1 || conflicts != 1 {
		t.Fatalf("connected=%d conflicts=%d", connected, conflicts)
	}
	providers, err := e.store.MessengerProviders(e.seat(e.rep))
	if err != nil || len(providers) != 1 {
		t.Fatalf("workspace providers=%v error=%v", providers, err)
	}
}

func TestServiceDisconnectRequiresAdministrationAndRevokesEveryPersonalGrant(t *testing.T) {
	e := setupSlackMemberLinks(t)
	providerAccounts(e)
	for _, user := range []struct {
		member string
		ctx    context.Context
	}{{"REP", e.seat(e.rep)}, {"OTHER", e.seat(e.other)}} {
		if _, err := e.store.ConnectMember(user.ctx, MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: user.member}, []byte("fixture-token")); err != nil {
			t.Fatal(err)
		}
	}
	e.exec(t, `UPDATE chat_member_grant SET status='reauth_required' WHERE user_id=$1`, e.other)
	if err := e.store.DisconnectProviderMembers(e.seat(e.rep), "slack", teamchat.Selection{}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("non-admin switched the service: %v", err)
	}
	admin := e.seatWith(e.rep, principal.RowScopeAll, map[string]principal.ObjectGrant{"channel_connection": {Read: true, Delete: true}})
	selection, err := e.store.SelectMessengerProvider(admin, "slack")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.DisconnectProviderMembers(admin, "slack", selection); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{e.seat(e.rep), e.seat(e.other)} {
		if _, _, err := e.store.OpenMember(ctx, "slack"); !errors.Is(err, messaging.ErrUnauthorized) {
			t.Fatalf("old grant still usable: %v", err)
		}
	}
	var revoked, audits int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM chat_member_grant WHERE status='disconnected' AND consent_version=2`).Scan(&revoked); err != nil || revoked != 2 {
		t.Fatalf("revoked=%d %v", revoked, err)
	}
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type='chat_member_grant' AND action='update'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("revocation audit=%d %v", audits, err)
	}
}

func TestAdministrativeSelectionPersistsBeforeConsentAndRejectsOldCallbacks(t *testing.T) {
	e := setupLinks(t)
	e.exec(t, `DELETE FROM channel_connection`)
	providerAccounts(e)
	admin := e.seatWith(e.rep, principal.RowScopeAll, map[string]principal.ObjectGrant{"channel_connection": {Read: true, Delete: true}})
	if _, err := e.store.SelectMessengerProvider(e.seat(e.rep), "slack"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("non-admin selection: %v", err)
	}
	if _, err := e.store.SelectMessengerProvider(admin, "slack"); err != nil {
		t.Fatal(err)
	}
	selected, err := e.store.SelectedMessengerProvider(e.seat(e.rep))
	if err != nil || selected != "slack" {
		t.Fatalf("selected=%q error=%v", selected, err)
	}
	if _, err := e.store.ConnectMember(e.seat(e.rep), MemberGrant{Provider: "teams", Workspace: "WORKSPACE", Member: "REP"}, []byte("fixture")); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("stale consent accepted: %v", err)
	}
	if _, err := e.store.ConnectMember(e.seat(e.rep), MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "REP"}, []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	selection, err := e.store.SelectMessengerProvider(admin, "teams")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.store.OpenMember(e.seat(e.rep), "slack"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("previous service remained usable: %v", err)
	}
	if err := e.store.DisconnectProviderMembers(admin, "slack", selection); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ConnectMember(e.seat(e.rep), MemberGrant{Provider: "teams", Workspace: "WORKSPACE", Member: "REP"}, []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	providers, err := e.store.MessengerProviders(e.seat(e.rep))
	if err != nil || len(providers) != 1 || providers[0] != "teams" {
		t.Fatalf("providers=%v error=%v", providers, err)
	}
	var audits, events int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_type='chat_service'`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='chat_service.selected'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if audits != 2 || events != 2 {
		t.Fatalf("selection trace audits=%d events=%d", audits, events)
	}
}

func TestSelectionVersionPreventsStaleCleanupAfterSwitchingBack(t *testing.T) {
	e := setupSlackMemberLinks(t)
	providerAccounts(e)
	ctx := e.seat(e.rep)
	admin := e.seatWith(e.rep, principal.RowScopeAll, map[string]principal.ObjectGrant{"channel_connection": {Read: true, Delete: true}})
	first, err := e.store.SelectMessengerProvider(admin, "slack")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "REP"}, []byte("fixture")); err != nil {
		t.Fatal(err)
	}
	stale, err := e.store.SelectMessengerProvider(admin, "teams")
	if err != nil {
		t.Fatal(err)
	}
	latest, err := e.store.SelectMessengerProvider(admin, "slack")
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func() error{
		func() error { return e.store.DisconnectProviderMembers(admin, "slack", stale) },
		func() error { return e.store.EndProviderLinks(admin, "slack", stale) },
		func() error { _, err := e.store.SelectMessengerProvider(admin, "", first); return err },
	} {
		if err := operation(); !errors.Is(err, apperrors.ErrVersionSkew) {
			t.Fatalf("stale operation accepted: %v", err)
		}
	}
	if _, _, err := e.store.OpenMember(ctx, "slack"); err != nil {
		t.Fatalf("current consent revoked: %v", err)
	}
	if _, err := e.store.SelectMessengerProvider(admin, "", latest); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.ConnectMember(ctx, MemberGrant{Provider: "slack", Workspace: "WORKSPACE", Member: "REP"}, []byte("late-callback")); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("disconnected service reopened: %v", err)
	}
}

func TestPersonalSlackProfileDoesNotRequireSharedInstallation(t *testing.T) {
	e := setupLinks(t)
	e.exec(t, `DELETE FROM channel_connection`)
	admin := e.seatWith(e.rep, principal.RowScopeAll, map[string]principal.ObjectGrant{"channel_connection": {Read: true, Delete: true}})
	selection, err := e.store.SelectMessengerProvider(admin, "slack")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.SetWorkspaceHistoryProfile(admin, selection, "internal", time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	profile, err := e.store.WorkspaceHistoryProfile(principal.WithActor(admin, principal.Principal{Type: principal.PrincipalSystem, ID: "system:chat_links"}), "slack")
	if err != nil || profile == nil || profile.Distribution != "internal" {
		t.Fatalf("personal profile unavailable: %+v %v", profile, err)
	}
}

func TestServiceSwitchEndsLinksAndRetainsThemForRetryUntilCleanupCompletes(t *testing.T) {
	e := setupLinks(t)
	no := false
	linked, err := e.store.Link(e.seat(e.rep), RecordDeal, e.deal, LinkRequest{Mode: ModeExisting, Channel: "team-acme", SummarizeHistory: &no})
	if err != nil {
		t.Fatal(err)
	}
	admin := e.seatWith(e.rep, principal.RowScopeAll, map[string]principal.ObjectGrant{"channel_connection": {Read: true, Delete: true}})
	selected, err := e.store.SelectMessengerProvider(admin, "slack")
	if err != nil {
		t.Fatal(err)
	}
	e.exec(t, `UPDATE channel_connection SET archived_at=now(),status='disconnected'`)
	providers, err := e.store.MessengerProviders(admin)
	if err != nil || !slices.Contains(providers, "dispact") {
		t.Fatalf("retry lost old links: %v %v", providers, err)
	}
	if err := e.store.EndProviderLinks(admin, "dispact", selected); err != nil {
		t.Fatal(err)
	}
	var ended bool
	if err := e.owner.QueryRow(context.Background(), `SELECT archived_at IS NOT NULL FROM chat_link WHERE id=$1`, linked.Link.ID).Scan(&ended); err != nil || !ended {
		t.Fatalf("old record occupied: %v %v", ended, err)
	}
	providers, err = e.store.MessengerProviders(admin)
	if err != nil || slices.Contains(providers, "dispact") {
		t.Fatalf("ended links still block service: %v %v", providers, err)
	}
}
