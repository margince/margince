// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The member that owns an extension's unattended feed.
//
// A feed filed under a colleague stops the day they are offboarded. A service
// member does not leave: an operator declares it, nobody signs in as it, and it
// is archived, never deleted. It is an `is_agent` row with no password, so
// sign-in refuses it. `service_extension` names the one unit it serves, and the
// consent check compares it so no other unit can act as it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/policy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// roleServiceIngest is the narrowest role that lets a unit's landing succeed:
// the activity, plus the contact and company the counterparty ladder may mint
// beside it. Nothing here reads or writes anywhere else.
const roleServiceIngest = "service_ingest"

// serviceMemberEmailDomain is a reserved TLD, so the synthetic address a service
// member needs for the unique email column can never be delivered to.
const serviceMemberEmailDomain = "members.invalid"

// ServiceMemberInput declares one service member.
type ServiceMemberInput struct {
	// Extension is the unit the member serves. The consent check refuses any
	// other unit.
	Extension   string
	DisplayName string
}

// DeclareServiceMember creates a service member for one extension unit.
// Installation admin only, human only: an agent passport may not hand out an
// identity that acts with nobody at the keyboard.
//
// It has a full seat type and is free of the licence seat limit. The seat meter
// skips `is_agent` rows, so no declaration can hit the ceiling. Its role,
// service_ingest, is created on first use.
func (s *Service) DeclareServiceMember(ctx context.Context, actor Identity, in ServiceMemberInput) (ids.UserID, error) {
	if err := requireInstallationAdmin(ctx, actor); err != nil {
		return ids.UserID{}, err
	}
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionCreate)
	if err != nil {
		return ids.UserID{}, err
	}
	if err := s.refuseUntilDescribed(ctx); err != nil {
		return ids.UserID{}, err
	}
	var member ids.UserID
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		roleID, err := EnsureServiceIngestRole(ctx, tx)
		if err != nil {
			return err
		}
		member = ids.From[ids.UserKind](ids.NewV7())
		address := fmt.Sprintf("service-%s@%s", member, serviceMemberEmailDomain)
		if _, err := tx.Exec(ctx,
			`INSERT INTO app_user (id, email, display_name, is_agent, service_extension, email_domain_folded)
			 VALUES ($1, $2, $3, true, $4, $5)`,
			member, address, in.DisplayName, in.Extension, owndomain.StoredOfAddress(address)); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`,
			roleID, member); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", "user", member.UUID, nil, map[string]any{
			"service_extension": in.Extension, roleEntity: roleServiceIngest,
			userAuditKeyStatus: userStatusActive,
		})
		return err
	})
	if err != nil {
		return ids.UserID{}, err
	}
	return member, nil
}

// ArchiveServiceMember retires a service member, the only way one leaves. The
// row stays, so its audit trail and secret still name something real. The feed
// stops because the live-member check no longer finds it.
func (s *Service) ArchiveServiceMember(ctx context.Context, actor Identity, member ids.UserID) error {
	if err := requireInstallationAdmin(ctx, actor); err != nil {
		return err
	}
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionDelete)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		var status string
		err := tx.QueryRow(ctx,
			`SELECT status FROM app_user
			  WHERE id = $1 AND service_extension IS NOT NULL AND archived_at IS NULL FOR UPDATE`,
			member).Scan(&status)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE app_user SET status = 'deactivated', archived_at = now() WHERE id = $1`,
			member); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", "user", member.UUID,
			map[string]any{userAuditKeyStatus: status},
			map[string]any{userAuditKeyStatus: userStatusDeactivated, roleFieldArchived: true})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, member.UUID, userDeactivatedPayload(member, actor.UserID, nil))
	})
}

// requireInstallationAdmin holds the literal-admin line. The user_admin grant
// is delegable, and a delegated holder must not mint an identity that acts
// without anyone present. Human-only for the same reason: the contract's
// `x-agent-access: human-only` declares it and the generated wrapper enforces
// nothing.
func requireInstallationAdmin(ctx context.Context, actor Identity) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	if !actor.hasRole(roleAdmin) {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// EnsureServiceIngestRole returns the service_ingest role's id, seeding it on
// first use. The no-op update is what lets RETURNING answer for a row that
// already exists.
//
// Exported for the compose integration suite. It cannot build the admin identity
// DeclareServiceMember needs. A hand-written copy of the grant could pass while
// the shipped role fails to land a record.
func EnsureServiceIngestRole(ctx context.Context, tx pgx.Tx) (ids.UUID, error) {
	var roleID ids.UUID
	err := tx.QueryRow(ctx,
		`INSERT INTO role (key, name, is_system, permissions) VALUES ($1, $2, true, $3)
		 ON CONFLICT (key) DO UPDATE SET key = EXCLUDED.key RETURNING id`,
		roleServiceIngest, "Service member (feed)", policy.MustDefaultJSON(roleServiceIngest)).Scan(&roleID)
	return roleID, err
}
