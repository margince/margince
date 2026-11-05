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
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// PartyReceiptSettlement retires delivery work without changing its successful receipt.
type PartyReceiptSettlement func(context.Context, ids.UUID) error

// WithPartyReceiptSettlement binds the delivery owner's completion marker.
func (s *Sink) WithPartyReceiptSettlement(settle PartyReceiptSettlement) *Sink {
	clone := *s
	clone.partyReceiptSettlement = settle
	return &clone
}

// PartyAuthority checks the capturing member both before admission and inside its write.
type PartyAuthority interface {
	EffectiveAuthority(context.Context, ids.UUID, ids.UUID) (authz.RBAC, principal.SeatType, error)
	EffectiveAuthorityTx(context.Context, pgx.Tx, ids.UUID) (authz.RBAC, principal.SeatType, error)
}

// WithPartyAuthority binds admission to current member authority.
func (s *Sink) WithPartyAuthority(authority PartyAuthority) *Sink {
	c := *s
	c.partyAuthority = authority
	return &c
}

type partySource struct {
	User            ids.UUID
	Provider        string
	Containers      []string
	ContainersKnown bool
	GovernedSend    bool
	Blocked         bool
	Addresses       []string
	Generation      int64
	Receipt         *ids.UUID
}

var errPartyAuthorityChanged = errors.New("capture: party authority changed during admission")

type admissionParty struct {
	Email string
	Name  string
}

// AdmitActivityParties derives counterparties only from currently admitted provider or send proof.
func (s *Sink) AdmitActivityParties(ctx context.Context, activity ids.ActivityID) error {
	if s.ensurer == nil || s.partyAuthority == nil {
		return nil
	}
	var sources []partySource
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		sources, err = s.partyAdmissionSources(ctx, tx, activity)
		return err
	})
	if err != nil {
		return err
	}
	var failed error
	for _, source := range sources {
		acting, settled, err := s.admitPartiesAs(ctx, activity, source)
		if err == nil && settled {
			err = s.settlePartyAdmission(acting, activity, source)
		}
		if err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				continue
			}
			failed = errors.Join(failed, err)
		}
	}
	return failed
}

func (s *Sink) admitPartiesAs(ctx context.Context, activity ids.ActivityID, source partySource) (context.Context, bool, error) {
	if source.Blocked {
		return ctx, true, nil
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ctx, false, apperrors.ErrPermissionDenied
	}
	rbac, seat, err := s.partyAuthority.EffectiveAuthority(ctx, ws, source.User)
	if err != nil {
		return ctx, false, err
	}
	if !seat.CanMutate() {
		return ctx, false, nil
	}
	acting := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalConnector, ID: connectorPrincipalID(source.Provider), UserID: source.User, OnBehalfOf: source.User, TeamIDs: rbac.TeamIDs, SeatType: seat, Permissions: rbac.Permissions})
	parties, ready, err := s.readAdmissionParties(acting, activity, source)
	if err != nil || !ready {
		return ctx, false, err
	}
	var failed error
	for _, party := range parties {
		if err := s.admitOneParty(acting, activity, source, party); err != nil {
			if errors.Is(err, errPartyAuthorityChanged) {
				return ctx, false, nil
			}
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				continue
			}
			failed = errors.Join(failed, err)
		}
	}
	return acting, failed == nil, failed
}

func (s *Sink) admitOneParty(ctx context.Context, activity ids.ActivityID, source partySource, party admissionParty) error {
	var rec connector.NormalizedRecord
	var decision counterpartyDecision
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activity.UUID); err != nil {
			return err
		}
		room, err := partyAdmissionHasRoomTx(ctx, tx, activity, 1)
		if err != nil || !room {
			return err
		}
		private, err := storedMessageIsPrivateTx(ctx, tx, activity, source.User)
		if err != nil || private {
			return err
		}
		if err := readPartyAdmissionRecord(ctx, tx, activity, source, &rec); err != nil {
			return err
		}
		cp := connector.Counterparty{Email: normalizeEmail(party.Email), DisplayName: party.Name, Domain: owndomain.OfAddress(party.Email)}
		subject, err := s.ladderSubjectTx(ctx, tx, connector.NormalizedRecord{Counterparty: cp, Addresses: []string{cp.Email}})
		if err != nil || subject.Email == "" {
			return err
		}
		excluded, err := excludedTx(ctx, tx, rec)
		if err != nil || excluded != "" {
			return err
		}
		decision, err = s.decideMailSubject(ctx, tx, rec, activity.UUID, cp)
		return err
	})
	if err != nil {
		return err
	}
	if !decision.create {
		return nil
	}
	cp := decision.subject
	outcome, err := s.ensurer.EnsureCounterparty(ctx, EnsureRequest{Admission: s.partyAdmissionGuard(activity, source, party), Email: cp.Email, DisplayName: cp.DisplayName, Domain: cp.Domain, OwnerID: decision.owner, ActivityID: activity.UUID, Source: captureSource(rec), CapturedBy: decision.capturedBy, SuppressCompany: decision.suppressCompany, Replied: decision.replied})
	if err != nil {
		return fmt.Errorf("admit a captured participant: %w", err)
	}
	pageProgressFrom(ctx).counted(ctx, outcome)
	return nil
}

func (s *Sink) admitCapturedParties(ctx context.Context, rec connector.NormalizedRecord, ref datasource.EntityRef) {
	if ref.Type != datasource.EntityActivity {
		return
	}
	if err := s.AdmitActivityParties(ctx, ids.From[ids.ActivityKind](ref.ID)); err != nil {
		s.logEnsureFault(ctx, rec, err)
	}
}

func (s *Sink) readAdmissionParties(ctx context.Context, activity ids.ActivityID, source partySource) ([]admissionParty, bool, error) {
	var parties []admissionParty
	ready := false
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
			return err
		}
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, activity.UUID); err != nil {
			return err
		}
		available, err := partyContainersAvailableTx(ctx, tx, source)
		if err != nil || !available {
			return err
		}
		private, err := storedMessageIsPrivateTx(ctx, tx, activity, source.User)
		if err != nil || private {
			return err
		}
		ready = true
		parties, err = eligibleAdmissionPartiesTx(ctx, tx, activity, source, false)
		return err
	})
	return parties, ready, err
}
