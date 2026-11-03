// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (s *Sink) counterpartyAdmission(original connector.NormalizedRecord, ref datasource.EntityRef, cp connector.Counterparty) func(context.Context, pgx.Tx, bool) error {
	return func(ctx context.Context, tx pgx.Tx, created bool) error {
		_, owner := capturePrincipal(ctx)
		source := partySource{User: owner, Provider: deliveringConnector(ctx), Containers: original.Containers, ContainersKnown: true}
		activity := ids.From[ids.ActivityKind](ref.ID)
		rec, err := s.guardAutomaticActivity(ctx, tx, activity, &source)
		if err != nil {
			return err
		}
		return s.guardAutomaticSubject(ctx, tx, activity, source, rec, cp, created)
	}
}

func (s *Sink) partyAdmissionGuard(activity ids.ActivityID, source partySource, party admissionParty) func(context.Context, pgx.Tx, bool) error {
	return func(ctx context.Context, tx pgx.Tx, created bool) error {
		currentSource := source
		rec, err := s.guardAutomaticActivity(ctx, tx, activity, &currentSource)
		if err != nil {
			return err
		}
		candidates, err := eligibleAdmissionPartiesTx(ctx, tx, activity, currentSource, true)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if normalizeEmail(candidate.Email) != normalizeEmail(party.Email) {
				continue
			}
			cp := connector.Counterparty{Email: normalizeEmail(candidate.Email), Domain: owndomain.OfAddress(candidate.Email)}
			return s.guardAutomaticSubject(ctx, tx, activity, currentSource, rec, cp, created)
		}
		return apperrors.ErrPermissionDenied
	}
}

func (s *Sink) guardAutomaticActivity(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source *partySource) (connector.NormalizedRecord, error) {
	return s.guardPartyActivity(ctx, tx, activity, source, true)
}

func (s *Sink) guardPartyActivity(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source *partySource, lock bool) (connector.NormalizedRecord, error) {
	var rec connector.NormalizedRecord
	args := []any{activity}
	var locked ids.UUID
	locking := ""
	if lock {
		locking = " FOR UPDATE"
	}
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT id FROM activity WHERE id=$%d AND archived_at IS NULL%s`, len(args), locking), args...).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return rec, apperrors.ErrNotFound
		}
		return rec, err
	}
	if lock {
		var err error
		ctx, err = s.partyActorTx(ctx, tx, source.User)
		if err != nil {
			return rec, err
		}
	}

	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return rec, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activity.UUID); err != nil {
		return rec, err
	}
	if err := guardPartySource(ctx, tx, activity, source, lock); err != nil {
		return rec, err
	}
	if err := readPartyAdmissionRecord(ctx, tx, activity, *source, &rec); err != nil {
		return rec, err
	}
	excluded, err := excludedTx(ctx, tx, rec)
	if err != nil {
		return rec, err
	}
	if excluded != "" {
		return rec, apperrors.ErrPermissionDenied
	}
	return rec, nil
}

func guardPartySource(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source *partySource, lock bool) error {
	private, err := storedMessageIsPrivateTx(ctx, tx, activity, source.User)
	if err != nil {
		return err
	}
	if private {
		return apperrors.ErrPermissionDenied
	}
	if err := refreshPartySourceTx(ctx, tx, activity, source, lock); err != nil {
		return err
	}
	if source.Blocked {
		return apperrors.ErrPermissionDenied
	}
	available, err := partyContainersAvailableTx(ctx, tx, *source)
	if err != nil {
		return err
	}
	if !available {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

func refreshPartySourceTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source *partySource, lock bool) error {
	if source.GovernedSend {
		return nil
	}
	args := []any{activity, source.User, source.Provider}
	locking := ""
	if lock {
		locking = " FOR SHARE"
	}
	query := fmt.Sprintf(`SELECT containers,containers_known,admission_blocked,policy_addresses FROM capture_party_source WHERE activity_id=$%d AND user_id=$%d AND provider=$%d%s`, len(args)-2, len(args)-1, len(args), locking)
	err := tx.QueryRow(ctx, query, args...).Scan(&source.Containers, &source.ContainersKnown, &source.Blocked, &source.Addresses)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrPermissionDenied
	}
	return err
}

func (s *Sink) guardAutomaticSubject(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source partySource, rec connector.NormalizedRecord, cp connector.Counterparty, created bool) error {
	subject, err := s.ladderSubjectTx(ctx, tx, connector.NormalizedRecord{Counterparty: cp, Addresses: []string{cp.Email}})
	if err != nil {
		return err
	}
	if subject.Email == "" {
		return apperrors.ErrPermissionDenied
	}
	if !created {
		return nil
	}
	room, err := partyAdmissionHasRoomTx(ctx, tx, activity, 0)
	if err != nil {
		return err
	}
	if !room {
		return apperrors.ErrPermissionDenied
	}
	if !s.recordWorthy(cp) {
		return apperrors.ErrPermissionDenied
	}
	fields, ok := rec.Fields.(ActivityFields)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	if fields.Kind == meetingKind {
		return s.guardMeetingParty(ctx, tx, activity, source, cp)
	}

	if fields.Direction == connector.DirectionOutbound {
		sent, err := partyWasWrittenToTx(ctx, tx, cp.Email)
		if err != nil {
			return err
		}
		if !sent {
			return apperrors.ErrPermissionDenied
		}
	}
	return nil
}

func partyContainersAvailableTx(ctx context.Context, tx pgx.Tx, source partySource) (bool, error) {
	if source.ContainersKnown {
		return true, nil
	}
	args := []any{source.User, source.Provider}
	var excluded bool
	err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM capture_exclusion e WHERE e.kind='container'
 AND (e.scope='workspace' OR e.user_id=$%d) AND split_part(e.value,':',1)=$%d)`, len(args)-1, len(args)), args...).Scan(&excluded)
	return !excluded, err
}

func partyAdmissionHasRoomTx(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, needed int) (bool, error) {
	var count int
	args := []any{activity}
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM activity_link WHERE activity_id=$%d`, len(args)), args...).Scan(&count); err != nil {
		return false, err
	}
	// The primary ensure has run; project and deal attribution still need their reserved links.
	return count+needed <= maxDerivedMeetingLinks-(postCommitLinkReservation-1), nil
}

func (s *Sink) partyActorTx(ctx context.Context, tx pgx.Tx, user ids.UUID) (context.Context, error) {
	if s.partyAuthority == nil {
		return ctx, errors.Join(apperrors.ErrPermissionDenied, errPartyAuthorityChanged)
	}
	rbac, seat, err := s.partyAuthority.EffectiveAuthorityTx(ctx, tx, user)
	if err != nil {
		return ctx, errors.Join(err, errPartyAuthorityChanged)
	}
	if !seat.CanMutate() {
		return ctx, errors.Join(apperrors.ErrPermissionDenied, errPartyAuthorityChanged)
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ctx, errors.Join(apperrors.ErrPermissionDenied, errPartyAuthorityChanged)
	}
	actor.Permissions, actor.TeamIDs, actor.SeatType = rbac.Permissions, rbac.TeamIDs, seat
	ctx = principal.WithActor(ctx, actor)
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return ctx, errors.Join(err, errPartyAuthorityChanged)
	}
	return ctx, nil
}

func (s *Sink) guardMeetingParty(ctx context.Context, tx pgx.Tx, activity ids.ActivityID, source partySource, cp connector.Counterparty) error {
	consumer, err := consumerMailSender(ctx, tx, cp.Domain)
	if err != nil {
		return err
	}
	if consumer {
		return apperrors.ErrPermissionDenied
	}
	parties, err := eligibleAdmissionPartiesTx(ctx, tx, activity, source, true)
	if err != nil {
		return err
	}
	for _, party := range parties {
		if normalizeEmail(party.Email) == cp.Email {
			return nil
		}
	}
	return apperrors.ErrPermissionDenied
}
