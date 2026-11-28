// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// VerifySource resolves a source URL only after current participant access is verified.
func (s *Store) VerifySource(ctx context.Context, provider string, connection ids.UUID, channel, root string) (string, error) {
	if _, err := memberActor(ctx); err != nil {
		return "", err
	}
	if !validSourceReference(provider, channel, root) {
		return "", messaging.ErrUnavailable
	}
	var workspace string
	var live bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT channel_id,status='connected' AND archived_at IS NULL FROM channel_connection WHERE id=@id AND provider=@provider AND account_id IS NULL AND parent_connection_id IS NULL`, pgx.StrictNamedArgs{"id": connection, providerKey: provider}).Scan(&workspace, &live)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", messaging.ErrUnavailable
	}
	if err != nil {
		return "", err
	}
	if provider == ProviderDispact && !live {
		return "", messaging.ErrUnavailable
	}
	workspace, err = sourceWorkspace(provider, workspace)
	if err != nil {
		return "", err
	}
	grant, client, err := s.OpenMember(ctx, provider)
	if err != nil {
		return "", err
	}
	if grant.Workspace != workspace {
		return "", messaging.ErrUnauthorized
	}
	reader, ok := client.(messaging.SourceReader)
	if !ok {
		return "", messaging.ErrUnsupported
	}
	url, err := reader.Source(ctx, channel, root)
	if err != nil {
		return "", err
	}
	current, err := s.MemberGrant(ctx, provider)
	if err != nil {
		return "", err
	}
	if !sameMemberConsent(current, grant) {
		return "", messaging.ErrUnauthorized
	}
	return url, nil
}

func sourceWorkspace(provider, workspace string) (string, error) {
	if provider == ProviderTeams {
		tenant, _, ok := strings.Cut(workspace, "/")
		if !ok {
			return "", messaging.ErrUnavailable
		}
		workspace = tenant
	}
	return workspace, nil
}

func sameMemberConsent(current, grant MemberGrant) bool {
	return current.ID == grant.ID && current.Version == grant.Version && current.Status == MemberConnected
}

func validSourceReference(provider, channel, root string) bool {
	return IsChatProvider(provider) && channel != "" && root != ""
}
