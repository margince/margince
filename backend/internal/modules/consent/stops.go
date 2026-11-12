// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// ListStops reads the live stops on a contact, each with the lift verdict this
// caller would get from Lift.
func (s *Store) ListStops(ctx context.Context, contactID ids.ContactID) ([]crmcontracts.ContactStop, error) {
	return s.stopsOfSubject(ctx, RecordInput{ContactID: contactID})
}

// ListLeadStops is ListStops for a lead.
func (s *Store) ListLeadStops(ctx context.Context, leadID ids.LeadID) ([]crmcontracts.ContactStop, error) {
	return s.stopsOfSubject(ctx, RecordInput{LeadID: leadID})
}

func (s *Store) stopsOfSubject(ctx context.Context, in RecordInput) ([]crmcontracts.ContactStop, error) {
	sub, err := consentSubject(in)
	if err != nil {
		return nil, err
	}
	if err := requireReadOnSubject(ctx, sub.entityType); err != nil {
		return nil, err
	}
	caller := lifterOf(ctx)
	asksLift := admitRestriction(ctx) == nil
	var out []crmcontracts.ContactStop
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureReadable(ctx, tx, sub.entityType, sub.id); err != nil {
			return err
		}
		admitted, err := mayLiftOn(ctx, tx, sub)
		if err != nil {
			return err
		}
		where, addressArgs, err := liveStopsOf(ctx, tx, sub)
		if err != nil {
			return err
		}
		// The colleague is named only while the carry chain still reaches the
		// row they typed. A chain whose first row is gone ends at a copy, whose
		// captured_by is whoever merged.
		rows, err := tx.Query(ctx, `
			WITH live AS (
			  SELECT s.id, s.kind, s.decided_by_level, s.source, s.recorded_at, s.address,
			         s.`+sub.column+` IS NULL AS pinned, s.purpose_id, s.recorded_through,
			         `+originColumnSQL("s", "captured_by")+` AS origin,
			         `+originColumnSQL("s", "source")+` AS origin_source
			    FROM communication_suppression s
			   WHERE `+where+`)
			SELECT live.id, live.kind, live.decided_by_level, live.source, live.recorded_at,
			       live.address, live.pinned, cp.key, live.recorded_through,
			       CASE WHEN live.recorded_through IN ($5, $7) AND NOT starts_with(live.origin_source, $6)
			            THEN u.display_name END,
			       notice.status, notice.reason, notice.created_at
			  FROM live
			  LEFT JOIN consent_purpose cp ON cp.id = live.purpose_id
			  LEFT JOIN passport p ON live.origin = $8 || p.id::text
			  LEFT JOIN app_user u ON live.origin = $4 || u.id::text OR u.id = p.on_behalf_of
			  LEFT JOIN LATERAL (
			       SELECT o.status, o.reason, o.created_at
			         FROM communication_suppression rs
			         JOIN comms_outbound o ON o.message_id LIKE `+liftNoticeLikeSQL("rs.id")+`
			        WHERE live.kind = '`+kindProcessingRestriction+`' AND rs.`+sub.column+` = $1
			          AND `+auth.LiveRestrictionSQL("rs")+`
			        ORDER BY o.created_at DESC LIMIT 1) notice ON true
			 ORDER BY live.recorded_at DESC, live.id`,
			append([]any{sub.id}, append(addressArgs,
				principal.HumanIDPrefix, string(commsauthz.RecordedByColleague), carriedSourceMark,
				string(commsauthz.RecordedByAgent), passportAgentPrefix)...)...)
		if err != nil {
			return fmt.Errorf("consent: reading the subject's stops: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ContactStop, error) {
			return scanStop(row, stopReader{caller: caller, admitted: admitted, asksLift: admitted && asksLift})
		})
		if err != nil {
			return fmt.Errorf("consent: reading the subject's stops: %w", err)
		}
		return nil
	})
	if out == nil {
		out = []crmcontracts.ContactStop{}
	}
	return out, err
}

// stopReader is what the caller may do with the stops it reads.
type stopReader struct {
	caller lifter
	// admitted is the caller half of a lift on this subject.
	admitted bool
	// asksLift is an admin signed in, who may lift a restriction with its notice.
	asksLift bool
}

func scanStop(row pgx.CollectableRow, reader stopReader) (crmcontracts.ContactStop, error) {
	var (
		id                             ids.UUID
		kind, decided, source, through string
		recordedAt                     time.Time
		address, purpose, name         *string
		pinned                         bool
		notice                         liftNoticeRow
	)
	if err := row.Scan(&id, &kind, &decided, &source, &recordedAt,
		&address, &pinned, &purpose, &through, &name,
		&notice.status, &notice.reason, &notice.queuedAt); err != nil {
		return crmcontracts.ContactStop{}, err
	}
	verdict := liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByPermission}
	if reader.admitted {
		verdict = judgeLift(reader.caller, stopFacts{
			kind: kind, decided: commsauthz.AuthorityLevel(decided),
			through: commsauthz.RecordedThrough(through), pinned: pinned,
		})
	}
	stop := crmcontracts.ContactStop{
		Id:                  openapi_types.UUID(id),
		Kind:                kind,
		Authority:           crmcontracts.ContactStopAuthority(decided),
		PurposeKey:          purpose,
		Address:             address,
		Reason:              reasonFromSource(source),
		RecordedAt:          recordedAt,
		RecordedByColleague: recordedForASeat(commsauthz.RecordedThrough(through)),
		RecordedByName:      name,
		MayLift:             verdict.may,
		Correction:          verdict.correction,
	}
	if !verdict.may {
		stop.LiftBlockedBy = &verdict.blockedBy
	}
	if kind == kindProcessingRestriction {
		stop.LiftNotice = notice.wire()
		mayAsk := reader.asksLift && !pinned && !notice.underway()
		stop.MayLiftRestriction = &mayAsk
	}
	return stop, nil
}

// liftNoticeRow is the newest delivery of a restriction's lift notice.
type liftNoticeRow struct {
	status, reason *string
	queuedAt       *time.Time
}

// wire answers where the notice is. A stop still listed with a sent notice is
// one whose lift the sweep has yet to apply.
func (n liftNoticeRow) wire() *crmcontracts.RestrictionLiftNotice {
	if n.status == nil || n.queuedAt == nil {
		return nil
	}
	status := crmcontracts.RestrictionLiftNoticeStatusQueued
	switch *n.status {
	case deliverySent:
		status = crmcontracts.RestrictionLiftNoticeStatusSent
	case deliveryParked:
		status = crmcontracts.RestrictionLiftNoticeStatusFailed
	}
	return &crmcontracts.RestrictionLiftNotice{Status: status, Reason: n.reason, QueuedAt: *n.queuedAt}
}

// underway is a notice that waits or went out, which a second lift must not
// duplicate (refuseLiftUnderwayTx).
func (n liftNoticeRow) underway() bool {
	return n.status != nil && (*n.status == deliveryPending || *n.status == deliverySent)
}

// The comms_outbound statuses the restriction lift reads.
const (
	deliveryPending = "pending"
	deliverySent    = "sent"
	deliveryParked  = "parked"
)

// reasonFromSource reads back what suppressionSource stored: the words after
// its prefix, on a carried copy too. A row another writer stored names its
// door, not anybody's words.
func reasonFromSource(source string) *string {
	words, ok := strings.CutPrefix(strings.TrimPrefix(source, carriedSourceMark), seatSourcePrefix+": ")
	if !ok || strings.TrimSpace(words) == "" {
		return nil
	}
	return &words
}
