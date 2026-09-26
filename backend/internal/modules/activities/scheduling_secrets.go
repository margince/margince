// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// WithMeetingVault binds encrypted management-link storage to the booking engine.
func (s *Store) WithMeetingVault(vault keyvault.Vault) *Store {
	clone := *s
	clone.meetingVault = vault
	return &clone
}

// WithMeetingVault binds encrypted management-link storage to the booking engine.
func (h Handlers) WithMeetingVault(vault keyvault.Vault) Handlers {
	h.store = h.store.WithMeetingVault(vault)
	return h
}

func (s *Store) sealMeetingLink(ctx context.Context, token string) (string, error) {
	return s.sealBookingLink(ctx, "manage-", token)
}

func (s *Store) sealBookingLink(ctx context.Context, prefix, token string) (string, error) {
	if err := s.publicOriginUsable(); err != nil {
		return "", err
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok || s.meetingVault == nil {
		return "", apperrors.ErrPermissionDenied
	}
	link := strings.TrimRight(s.publicBaseURL, "/") + "/#/book/" + prefix + token
	ref, err := s.meetingVault.Put(ctx, ids.From[ids.WorkspaceKind](workspace), []byte(link))
	return string(ref), err
}

func (s *Store) openMeetingLink(ctx context.Context, ref string) (string, error) {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok || s.meetingVault == nil {
		return "", apperrors.ErrPermissionDenied
	}
	secret, err := s.meetingVault.Get(ctx, ids.From[ids.WorkspaceKind](workspace), keyvault.Ref(ref))
	return string(secret), err
}

func (s *Store) deleteMeetingLink(ctx context.Context, ref string) error {
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok || s.meetingVault == nil {
		return apperrors.ErrPermissionDenied
	}
	return s.meetingVault.Delete(ctx, ids.From[ids.WorkspaceKind](workspace), keyvault.Ref(ref))
}

// The proposal remains a recipient capability after use, so reloading can recover
// a booking whose response was lost without creating another invitation.
func (s *Store) recoverProposalInvitation(ctx context.Context, proposal proposalRow) (crmcontracts.MeetingInvitation, error) {
	if proposal.Invitation == nil {
		return crmcontracts.MeetingInvitation{}, apperrors.ErrNotFound
	}
	row, err := s.scopedInvitation(ctx, proposal.Host, *proposal.Invitation)
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	link, err := s.openMeetingLink(ctx, row.ManagementRef)
	if err != nil {
		return crmcontracts.MeetingInvitation{}, err
	}
	_, token, ok := strings.Cut(link, "/#/book/manage-")
	if !ok || len(token) != 43 {
		return crmcontracts.MeetingInvitation{}, apperrors.ErrNotFound
	}
	out := invitationView(row)
	out.CalendarUrl = nil
	out.ManagementToken = &token
	return out, nil
}
