// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const auditProposalStatus = "proposal_status"

type proposalRow struct {
	LinkRef    string
	ID         ids.UUID
	Host       ids.UserID
	Request    crmcontracts.MeetingProposalRequest
	Expires    time.Time
	Used       *time.Time
	Invitation *ids.UUID
}
type proposalLink struct {
	ID      ids.UUID  `json:"id"`
	URL     string    `json:"url"`
	Expires time.Time `json:"expires_at"`
}

// CreateProposal binds an expiring one-use link to a contact and optional times.
func (s *Store) CreateProposal(ctx context.Context, in crmcontracts.MeetingProposalRequest) (proposalLink, error) {
	host, err := schedulingHost(ctx)
	if err != nil {
		return proposalLink{}, err
	}
	expires, err := s.validateProposal(ctx, host, in)
	if err != nil {
		return proposalLink{}, err
	}
	token, hash, err := invitationToken()
	if err != nil {
		return proposalLink{}, err
	}
	ref, err := s.sealBookingLink(ctx, "proposal-", token)
	if err != nil {
		return proposalLink{}, err
	}
	var id ids.UUID
	err = s.tx(ctx, func(tx pgx.Tx) error {
		subject := "Meeting proposed: " + in.Subject
		row, _, err := s.LogActivityTx(ctx, tx, LogActivityInput{Kind: "note", Subject: &subject, Body: &in.Description, Links: []ActivityLinkInput{{EntityType: linkEntityContact, EntityID: ids.UUID(in.ContactId)}}, Source: sourceManual})
		if err != nil {
			return err
		}
		id = ids.UUID(row.Id)
		args := schedulingArgs{}
		values := []string{args.add(id), args.add(host), args.add(in), args.add(hash), args.add(expires), args.add(ref)}
		if _, err := tx.Exec(ctx, `INSERT INTO meeting_proposal(activity_id,host_user_id,request,token_hash,expires_at,link_ref) VALUES(`+strings.Join(values, ",")+`)`, args...); err != nil {
			return err
		}
		return auditProposal(ctx, tx, id, "created")
	})
	if err != nil {
		return proposalLink{}, errors.Join(err, s.deleteMeetingLink(ctx, ref))
	}
	return proposalLink{ID: id, URL: strings.TrimRight(s.publicBaseURL, "/") + "/#/book/proposal-" + token, Expires: expires}, err
}

// ResolveProposalToken rejects expired capabilities without revealing recipient details.
func (s *Store) ResolveProposalToken(ctx context.Context, token string) (ids.UUID, ids.UserID, error) {
	if len(token) != 43 {
		return ids.Nil, ids.UserID{}, apperrors.ErrNotFound
	}
	sum := sha256.Sum256([]byte(token))
	var id ids.UUID
	var host ids.UserID
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		args := schedulingArgs{}
		return tx.QueryRow(ctx, `SELECT p.activity_id,p.host_user_id FROM meeting_proposal p JOIN activity a ON a.id=p.activity_id WHERE p.token_hash=`+args.add(hex.EncodeToString(sum[:]))+` AND a.archived_at IS NULL AND p.expires_at>`+args.add(s.now()), args...).Scan(&id, &host)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apperrors.ErrNotFound
	}
	return id, host, err
}

func (s *Store) readProposal(ctx context.Context, id ids.UUID) (proposalRow, error) {
	var row proposalRow
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := schedulingArgs{}
		return tx.QueryRow(ctx, `SELECT activity_id,host_user_id,request,expires_at,used_at,invitation_id,link_ref FROM meeting_proposal WHERE activity_id=`+args.add(id), args...).Scan(&row.ID, &row.Host, &row.Request, &row.Expires, &row.Used, &row.Invitation, &row.LinkRef)
	})
	return row, err
}

func (s *Store) lockOpenProposal(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	args := schedulingArgs{}
	var usable bool
	err := tx.QueryRow(ctx, `SELECT used_at IS NULL AND expires_at>`+args.add(s.now())+` FROM meeting_proposal WHERE activity_id=`+args.add(id)+` FOR UPDATE`, args...).Scan(&usable)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !usable {
		return apperrors.ErrConflict
	}
	return nil
}

func consumeProposal(ctx context.Context, tx pgx.Tx, id, invitation ids.UUID) error {
	args := schedulingArgs{}
	if _, err := tx.Exec(ctx, `UPDATE meeting_proposal SET used_at=now(),invitation_id=`+args.add(invitation)+` WHERE activity_id=`+args.add(id), args...); err != nil {
		return err
	}
	return auditProposal(ctx, tx, id, "used")
}

func auditProposal(ctx context.Context, tx pgx.Tx, id ids.UUID, status string) error {
	var audit ids.UUID
	var err error
	if status == "created" {
		audit, err = storekit.AuditEvent(ctx, tx, "create", "activity", id, map[string]any{auditProposalStatus: status})
	} else {
		audit, err = storekit.Audit(ctx, tx, "update", "activity", id, map[string]any{auditProposalStatus: "created"}, map[string]any{auditProposalStatus: status})
	}
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, audit, id, crmcontracts.InternalEventMeetingProposalUpdated{Status: status})
}

func (s *Store) validateProposalOptions(ctx context.Context, host ids.UserID, in crmcontracts.MeetingProposalRequest) (time.Time, error) {
	expires := s.now().Add(7 * 24 * time.Hour)
	var last time.Time
	for _, option := range in.Options {
		if option.End.Sub(option.Start) != time.Duration(in.DurationMinutes)*time.Minute {
			return time.Time{}, errAvailabilityDurationOutOfRange
		}
		if _, err := s.validateInvitationTime(ctx, host, option.Start, option.End); err != nil {
			return time.Time{}, err
		}
		if option.Start.After(last) {
			last = option.Start
		}
	}
	if !last.IsZero() && last.Before(expires) {
		expires = last
	}
	return expires, nil
}

// ProposalLink recovers a host's capability from the vault after a lost response.
func (s *Store) ProposalLink(ctx context.Context, id ids.UUID) (proposalLink, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return proposalLink{}, err
	}
	if err := s.tx(ctx, func(tx pgx.Tx) error {
		_, err := readActivity(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
		return err
	}); err != nil {
		return proposalLink{}, err
	}
	row, err := s.readProposal(ctx, id)
	if err != nil {
		return proposalLink{}, err
	}
	host, err := schedulingHost(ctx)
	if err != nil {
		return proposalLink{}, err
	}
	if row.Host != host {
		return proposalLink{}, apperrors.ErrNotFound
	}
	link, err := s.openMeetingLink(ctx, row.LinkRef)
	return proposalLink{ID: row.ID, URL: link, Expires: row.Expires}, err
}

func (s *Store) validateProposal(ctx context.Context, host ids.UserID, in crmcontracts.MeetingProposalRequest) (time.Time, error) {
	if err := s.publicOriginUsable(); err != nil {
		return time.Time{}, err
	}
	if err := validateInvitation(crmcontracts.MeetingInvitationRequest{ContactId: in.ContactId, AttendeeEmail: in.AttendeeEmail, Subject: in.Subject, Description: in.Description, Location: in.Location}); err != nil {
		return time.Time{}, err
	}
	if in.DurationMinutes < 15 || in.DurationMinutes > 480 || len(in.Options) > 3 {
		return time.Time{}, errAvailabilityDurationOutOfRange
	}
	profile, err := s.hostSchedulingProfile(ctx, host)
	if err != nil {
		return time.Time{}, err
	}
	if s.calendar == nil {
		return time.Time{}, apperrors.ErrPermissionDenied
	}
	if err := s.calendar.Check(ctx, host, string(profile.Provider)); err != nil {
		return time.Time{}, err
	}
	if err := s.calendar.CheckRecipient(ctx, host, ids.UUID(in.ContactId), string(in.AttendeeEmail)); err != nil {
		return time.Time{}, err
	}
	return s.validateProposalOptions(ctx, host, in)
}
