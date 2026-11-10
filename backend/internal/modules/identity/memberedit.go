// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// MemberEdit binds editable account settings to the revision the administrator saw.
type MemberEdit struct {
	Name  string
	Email string
	Role  *string
	Teams *[]ids.UUID
	// LegalEntity moves the member to that entity; nil keeps their entity.
	LegalEntity       *ids.LegalEntityID
	ExpectedUpdatedAt time.Time
}

func validateDisplayName(name string) error {
	if !values.HasVisibleText(name) || utf8.RuneCountInString(name) > displayNameMaxRunes {
		return &InvalidDisplayNameError{MaxRunes: displayNameMaxRunes}
	}
	return nil
}

// UpdateMember keeps profile and access edits in one transaction.
func (s *Service) UpdateMember(ctx context.Context, actor Identity, member ids.UserID, in MemberEdit) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionUpdate)
	if err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if err := validateDisplayName(in.Name); err != nil {
		return err
	}
	email, err := values.ParseEmail(in.Email)
	if err != nil {
		return err
	}
	in.Email = email.String()
	if in.Teams != nil && len(*in.Teams) > 2000 {
		return &values.ParseError{Field: fieldTeamIDs, Code: codeInvalid, Message: "choose at most 2000 teams"}
	}
	edit := func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
		if err := lockMemberAddressClaims(ctx, tx, member, in.Email); err != nil {
			return err
		}
		if err := refuseUnlessCallerOutranksTarget(ctx, tx, actor, member, reachTakeover); err != nil {
			return err
		}
		if err := refuseAgentEmailChange(ctx, tx, member, in.Email); err != nil {
			return err
		}
		if err := s.editMemberIdentityTx(ctx, tx, actor, member, in); err != nil {
			return err
		}
		if in.Role != nil {
			if err := s.changeUserRoleTx(ctx, tx, actor, member, *in.Role); err != nil {
				return err
			}
		}
		if in.Teams != nil {
			if err := s.replaceMemberTeamsTx(ctx, tx, actor, member.UUID, uniqueTeams(*in.Teams)); err != nil {
				return err
			}
		}
		return s.placeMember(ctx, tx, in.LegalEntity, member)
	}
	return storekit.RetryLockCycles(ctx, func() error { return s.db.Tx(ctx, edit) })
}

// lockMemberAddressClaims takes the seat-address claim lock on the member's
// login and on any address it is about to become, before any row lock. The
// authorization lock the caller holds keeps the login read here current.
func lockMemberAddressClaims(ctx context.Context, tx pgx.Tx, member ids.UserID, next ...string) error {
	var login string
	err := tx.QueryRow(ctx, `SELECT email FROM app_user WHERE id = $1`, member).Scan(&login)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("identity: reading the member's login: %w", err)
	}
	return storekit.LockSeatAddressClaims(ctx, tx, append(next, login)...)
}

func (s *Service) editMemberIdentityTx(ctx context.Context, tx pgx.Tx, actor Identity, member ids.UserID, in MemberEdit) error {
	var email, name string
	var updated time.Time
	var agent bool
	err := tx.QueryRow(ctx, `SELECT email, display_name, updated_at, is_agent FROM app_user
  WHERE id = @member AND archived_at IS NULL AND removed_at IS NULL FOR UPDATE`, pgx.StrictNamedArgs{memberParameter: member}).Scan(&email, &name, &updated, &agent)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if agent {
		return errAgentSeatHoldsNoRole
	}
	if !updated.Equal(in.ExpectedUpdatedAt) {
		return apperrors.ErrVersionSkew
	}
	if email == in.Email && name == in.Name {
		return nil
	}
	if email != in.Email {
		if err := requireUnusedMemberEmail(ctx, tx, member); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE app_user SET email = @email, display_name = @name, email_domain_folded = @domain WHERE id = @member`, pgx.StrictNamedArgs{memberParameter: member, memberEmailField: in.Email, memberNameArgument: in.Name, "domain": owndomain.StoredOfAddress(in.Email)}); err != nil {
		if storekit.IsUniqueViolation(err) {
			return errEmailTaken
		}
		return err
	}
	if email != in.Email {
		if err := endCredentialAuthority(ctx, tx, member, "account_email_changed"); err != nil {
			return err
		}
	}
	auditID, err := storekit.Audit(ctx, tx, "update", "user", member.UUID,
		map[string]string{memberEmailField: email, displayNameField: name}, map[string]string{memberEmailField: in.Email, displayNameField: in.Name})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, member.UUID, crmcontracts.PublicEventUserUpdated{
		UserId: openapi_types.UUID(member.UUID), By: openapi_types.UUID(actor.UserID.UUID),
	})
}

func (s *Service) replaceMemberTeamsTx(ctx context.Context, tx pgx.Tx, actor Identity, member ids.UUID, teams []ids.UUID) error {
	current, err := loadLiveTeams(ctx, tx, []ids.UUID{member})
	if err != nil {
		return err
	}
	wanted := make(map[ids.UUID]bool, len(teams))
	for _, team := range teams {
		wanted[team] = true
	}
	for _, team := range current[member] {
		if !wanted[team.UUID] {
			if err := s.setTeamMemberTx(ctx, tx, actor, team.UUID, member, false); err != nil {
				return err
			}
		}
	}
	for _, team := range teams {
		already := false
		for _, held := range current[member] {
			if held.UUID == team {
				already = true
			}
		}
		if already {
			continue
		}
		if err := s.setTeamMemberTx(ctx, tx, actor, team, member, true); err != nil {
			return err
		}
	}
	return nil
}
