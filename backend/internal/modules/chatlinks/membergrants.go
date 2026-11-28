// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/teamchat"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MemberGrantStatus distinguishes usable consent from disconnection and required repair.
type MemberGrantStatus string

// Grant states distinguish usable personal consent from disconnection and reauthorization.
const (
	MemberConnected               MemberGrantStatus = "connected"
	MemberDisconnected            MemberGrantStatus = "disconnected"
	MemberReauthorizationRequired MemberGrantStatus = "reauth_required"
)

// MemberGrant pins personal consent to one local member and native identity.
type MemberGrant struct {
	ID         ids.UUID
	UserID     ids.UUID
	Provider   string
	Workspace  string
	Member     string
	Name       string
	Address    string
	Status     MemberGrantStatus
	Version    int64
	credential string
}

// MemberAuthority holds the human's live seat and grants until tx ends.
type MemberAuthority func(context.Context, pgx.Tx) (context.Context, error)

// WithMemberAuthority rebuilds current member authority before personal provider operations.
func (s *Store) WithMemberAuthority(authority MemberAuthority) *Store {
	s.memberAuthority = authority
	return s
}

func (s *Store) readMemberTx(ctx context.Context, tx pgx.Tx) (context.Context, error) {
	if s.memberAuthority == nil {
		return nil, messaging.ErrUnsupported
	}
	live, err := s.memberAuthority(ctx, tx)
	if err != nil {
		return nil, err
	}
	if _, err := memberActor(live); err != nil {
		return nil, err
	}
	return live, nil
}

func (s *Store) holdMemberTx(ctx context.Context, tx pgx.Tx) (context.Context, error) {
	live, err := s.readMemberTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	if _, err := memberMutationActor(live); err != nil {
		return nil, err
	}
	return live, nil
}

// MemberRefresh rotates credentials while preserving their stored grant binding.
type MemberRefresh func(context.Context, MemberGrant, []byte) ([]byte, error)

// WithMemberRefresh injects credential rotation without changing personal grant ownership.
func (s *Store) WithMemberRefresh(refresh MemberRefresh) *Store { s.memberRefresh = refresh; return s }

// MemberClient opens a personal client from its verified grant and sealed credential.
type MemberClient func(context.Context, MemberGrant, []byte) (messaging.Client, error)

// WithMessenger keeps personal provider clients behind sealed credentials and record authority.
func (s *Store) WithMessenger(vault func() keyvault.Vault, client MemberClient) *Store {
	s.WithVault(vault)
	s.memberClient = client
	return s
}

func memberActor(ctx context.Context) (ids.UUID, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return ids.Nil, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return actor.UserID, nil
}

func memberMutationActor(ctx context.Context) (ids.UUID, error) {
	user, err := memberActor(ctx)
	if err != nil {
		return ids.Nil, err
	}
	if !auth.SeatAdmitsWrites(ctx) {
		return ids.Nil, apperrors.ErrSeatTierInsufficient
	}
	return user, nil
}

func validProvider(provider string) bool {
	return provider == "dispact" || provider == "slack" || provider == "teams"
}

const memberGrantColumns = "id,user_id,provider,provider_workspace,provider_user,address,status,consent_version,credential_ref"

func scanMemberGrant(row pgx.Row) (MemberGrant, error) {
	var grant MemberGrant
	err := row.Scan(&grant.ID, &grant.UserID, &grant.Provider, &grant.Workspace, &grant.Member, &grant.Address, &grant.Status, &grant.Version, &grant.credential)
	return grant, err
}

// MemberGrant pins personal consent to one local member and native identity.
func (s *Store) MemberGrant(ctx context.Context, provider string) (MemberGrant, error) {
	user, err := memberActor(ctx)
	if err != nil {
		return MemberGrant{}, err
	}
	if !validProvider(provider) {
		return MemberGrant{}, apperrors.ErrInvalidArgument
	}
	var grant MemberGrant
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.readMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		if err := teamchat.Check(ctx, tx, provider); err != nil {
			return err
		}
		grant, err = scanMemberGrant(tx.QueryRow(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE user_id=@user AND provider=@provider", pgx.StrictNamedArgs{userKey: user, providerKey: provider}))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return MemberGrant{}, messaging.ErrUnauthorized
	}
	return grant, err
}

//nolint:ireturn // The injected vault can use different key providers.
func (s *Store) memberVault() (keyvault.Vault, error) {
	if s.vault == nil || s.memberClient == nil {
		return nil, messaging.ErrUnsupported
	}
	vault := s.vault()
	if vault == nil {
		return nil, messaging.ErrUnsupported
	}
	return vault, nil
}

func grantImage(grant MemberGrant) map[string]any {
	return map[string]any{"user_id": grant.UserID, providerKey: grant.Provider, "status": grant.Status, "credential_reference_set": grant.credential != ""}
}

// ConnectMember accepts credentials only after the provider proves the selected
// identity. Secret bytes are sealed and never enter the row or its audit image.
func (s *Store) ConnectMember(ctx context.Context, grant MemberGrant, secret []byte) (MemberGrant, error) {
	user, err := memberMutationActor(ctx)
	if err != nil {
		return MemberGrant{}, err
	}
	if !validProvider(grant.Provider) || grant.Workspace == "" || grant.Member == "" || len(secret) == 0 {
		return MemberGrant{}, apperrors.ErrInvalidArgument
	}
	vault, err := s.memberVault()
	if err != nil {
		return MemberGrant{}, err
	}
	grant.UserID = user
	client, err := s.memberClient(ctx, grant, secret)
	if err != nil {
		return MemberGrant{}, err
	}
	identity, err := client.Identity(ctx)
	if err != nil {
		return MemberGrant{}, err
	}
	if !memberIdentityMatches(grant, identity) {
		return MemberGrant{}, messaging.ErrUnauthorized
	}
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return MemberGrant{}, err
	}
	ref, err := vault.Put(ctx, ws, secret)
	if err != nil {
		return MemberGrant{}, err
	}
	var previous MemberGrant
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var writeErr error
		grant, previous, writeErr = s.connectMemberTx(ctx, tx, grant, ref)
		return writeErr
	})
	if err != nil {
		keyvault.DeleteDetached(ctx, vault, slog.Default(), ws.UUID, ref, "messenger connect rollback")
		if _, duplicate := storekit.UniqueViolation(err); duplicate {
			return MemberGrant{}, apperrors.ErrConflict
		}
		return MemberGrant{}, err
	}
	keyvault.DeleteDetached(ctx, vault, slog.Default(), ws.UUID, keyvault.Ref(previous.credential), "messenger credential replacement")

	return grant, nil
}

// OpenMember revalidates consent and installation binding before returning a personal client.
//
//nolint:ireturn // Personal clients implement one consent protocol across the supported providers.
func (s *Store) OpenMember(ctx context.Context, provider string) (MemberGrant, messaging.Client, error) {
	grant, err := s.MemberGrant(ctx, provider)
	if err != nil {
		return MemberGrant{}, nil, err
	}
	if grant.Status != MemberConnected {
		return MemberGrant{}, nil, messaging.ErrUnauthorized
	}
	vault, err := s.memberVault()
	if err != nil {
		return MemberGrant{}, nil, err
	}
	secret, err := s.memberSecret(ctx, grant, vault)
	if err != nil {
		return MemberGrant{}, nil, err
	}
	client, err := s.memberClient(ctx, grant, secret)
	if err != nil {
		return MemberGrant{}, nil, err
	}
	identity, err := client.Identity(ctx)
	if err != nil {
		return MemberGrant{}, nil, err
	}
	if !memberIdentityMatches(grant, identity) {
		return MemberGrant{}, nil, messaging.ErrUnauthorized
	}
	grant.Name = identity.Member.Name
	return grant, client, nil
}

// DisconnectMember revokes local personal consent before retiring its credential.
func (s *Store) DisconnectMember(ctx context.Context, provider string) error {
	user, err := memberMutationActor(ctx)
	if err != nil {
		return err
	}
	if !validProvider(provider) {
		return apperrors.ErrInvalidArgument
	}
	ws, err := s.db.Workspace(ctx)
	if err != nil {
		return err
	}
	var credential string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.holdMemberTx(ctx, tx)
		if err != nil {
			return err
		}
		before, err := scanMemberGrant(tx.QueryRow(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE user_id=@user AND provider=@provider FOR UPDATE", pgx.StrictNamedArgs{userKey: user, providerKey: provider}))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		credential = before.credential
		return disconnectMemberGrantTx(ctx, tx, before, "member_disconnect")
	})
	if err != nil {
		return err
	}
	var vault keyvault.Vault
	if s.vault != nil {
		vault = s.vault()
	}
	keyvault.DeleteDetached(ctx, vault, slog.Default(), ws.UUID, keyvault.Ref(credential), "messenger disconnect")
	return nil
}

func holdMemberConsentTx(ctx context.Context, tx pgx.Tx, grant MemberGrant) error {
	current, err := scanMemberGrant(tx.QueryRow(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE id=@id AND user_id=@user FOR SHARE", pgx.StrictNamedArgs{"id": grant.ID, userKey: grant.UserID}))
	if errors.Is(err, pgx.ErrNoRows) {
		return messaging.ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if current.Status != "connected" || current.Version != grant.Version {
		return messaging.ErrUnauthorized
	}
	return nil
}

func memberIdentityMatches(grant MemberGrant, identity messaging.Identity) bool {
	return identity.Workspace == grant.Workspace && identity.Member.ID == grant.Member && identity.Member.Active && !identity.Member.External && !identity.Member.Bot
}

func (s *Store) connectMemberTx(ctx context.Context, tx pgx.Tx, grant MemberGrant, ref keyvault.Ref) (MemberGrant, MemberGrant, error) {
	ctx, err := s.holdMemberTx(ctx, tx)
	if err != nil {
		return MemberGrant{}, MemberGrant{}, err
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended(@key,0))", pgx.StrictNamedArgs{keyKey: "chat-member-grant:" + grant.UserID.String() + ":" + grant.Provider}); err != nil {
		return MemberGrant{}, MemberGrant{}, err
	}
	if err := teamchat.Admit(ctx, tx, grant.Provider); err != nil {
		return MemberGrant{}, MemberGrant{}, err
	}
	before, readErr := scanMemberGrant(tx.QueryRow(ctx, "SELECT "+memberGrantColumns+" FROM chat_member_grant WHERE user_id=@user AND provider=@provider FOR UPDATE", pgx.StrictNamedArgs{userKey: grant.UserID, providerKey: grant.Provider}))
	if readErr != nil && !errors.Is(readErr, pgx.ErrNoRows) {
		return MemberGrant{}, MemberGrant{}, readErr
	}
	grant, err = scanMemberGrant(tx.QueryRow(ctx, `INSERT INTO chat_member_grant
   (user_id,provider,provider_workspace,provider_user,address,credential_ref,status)
   VALUES(@user,@provider,@workspace,@member,@address,@credential,'connected')
   ON CONFLICT(user_id,provider) DO UPDATE SET provider_workspace=EXCLUDED.provider_workspace,
   provider_user=EXCLUDED.provider_user,address=EXCLUDED.address,credential_ref=EXCLUDED.credential_ref,status='connected',consent_version=chat_member_grant.consent_version+1
   RETURNING `+memberGrantColumns, pgx.StrictNamedArgs{userKey: grant.UserID, providerKey: grant.Provider, "workspace": grant.Workspace, "member": grant.Member, "address": grant.Address, credentialKey: ref}))
	if err != nil {
		return MemberGrant{}, MemberGrant{}, err
	}
	action := updateKey
	if before.ID.IsZero() {
		action = ModeCreate
	}
	_, err = storekit.Audit(ctx, tx, action, "chat_member_grant", grant.ID, grantImage(before), grantImage(grant))
	return grant, before, err
}
