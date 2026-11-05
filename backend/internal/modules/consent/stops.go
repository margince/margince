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
			       CASE WHEN live.recorded_through = $5 AND NOT starts_with(live.origin_source, $6)
			            THEN u.display_name END
			  FROM live
			  LEFT JOIN consent_purpose cp ON cp.id = live.purpose_id
			  LEFT JOIN app_user u ON live.origin = $4 || u.id::text
			 ORDER BY live.recorded_at DESC, live.id`,
			append([]any{sub.id}, append(addressArgs,
				principal.HumanIDPrefix, string(commsauthz.RecordedByColleague), carriedSourceMark)...)...)
		if err != nil {
			return fmt.Errorf("consent: reading the subject's stops: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ContactStop, error) {
			return scanStop(row, caller, admitted)
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

func scanStop(row pgx.CollectableRow, caller lifter, admitted bool) (crmcontracts.ContactStop, error) {
	var (
		id                             ids.UUID
		kind, decided, source, through string
		recordedAt                     time.Time
		address, purpose, name         *string
		pinned                         bool
	)
	if err := row.Scan(&id, &kind, &decided, &source, &recordedAt,
		&address, &pinned, &purpose, &through, &name); err != nil {
		return crmcontracts.ContactStop{}, err
	}
	verdict := liftVerdict{blockedBy: crmcontracts.ContactStopLiftBlockedByPermission}
	if admitted {
		verdict = judgeLift(caller, stopFacts{
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
		RecordedByColleague: commsauthz.RecordedThrough(through) == commsauthz.RecordedByColleague,
		RecordedByName:      name,
		MayLift:             verdict.may,
		Correction:          verdict.correction,
	}
	if !verdict.may {
		stop.LiftBlockedBy = &verdict.blockedBy
	}
	return stop, nil
}

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
