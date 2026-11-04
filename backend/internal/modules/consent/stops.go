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
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return nil, err
	}
	caller := authorityOf(ctx)
	var out []crmcontracts.ContactStop
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureReadable(ctx, tx, entityContact, contactID.UUID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			WITH live AS (
			  SELECT s.id, s.kind, s.decided_by_level, s.source, s.recorded_at, s.address,
			         s.contact_id IS NULL AS pinned, s.purpose_id,
			         `+originCapturedBySQL("s")+` AS origin
			    FROM communication_suppression s
			   WHERE `+contactLiveStopsSQL+`)
			SELECT live.id, live.kind, live.decided_by_level, live.source, live.recorded_at,
			       live.address, live.pinned, cp.key, live.origin, u.display_name
			  FROM live
			  LEFT JOIN consent_purpose cp ON cp.id = live.purpose_id
			  LEFT JOIN app_user u ON live.origin = $2 || u.id::text
			 ORDER BY live.recorded_at DESC, live.id`,
			contactID, principal.HumanIDPrefix)
		if err != nil {
			return fmt.Errorf("consent: reading the contact's stops: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ContactStop, error) {
			return scanStop(row, caller)
		})
		if err != nil {
			return fmt.Errorf("consent: reading the contact's stops: %w", err)
		}
		return nil
	})
	if out == nil {
		out = []crmcontracts.ContactStop{}
	}
	return out, err
}

func scanStop(row pgx.CollectableRow, caller commsauthz.AuthorityLevel) (crmcontracts.ContactStop, error) {
	var (
		id                     ids.UUID
		kind, decided, source  string
		origin                 string
		recordedAt             time.Time
		address, purpose, name *string
		pinned                 bool
	)
	if err := row.Scan(&id, &kind, &decided, &source, &recordedAt,
		&address, &pinned, &purpose, &origin, &name); err != nil {
		return crmcontracts.ContactStop{}, err
	}
	verdict := judgeLift(caller, stopFacts{
		kind: kind, decided: commsauthz.AuthorityLevel(decided), origin: origin, pinned: pinned,
	})
	stop := crmcontracts.ContactStop{
		Id:                  openapi_types.UUID(id),
		Kind:                kind,
		Authority:           crmcontracts.ContactStopAuthority(decided),
		PurposeKey:          purpose,
		Address:             address,
		Reason:              reasonFromSource(source),
		RecordedAt:          recordedAt,
		RecordedByColleague: !principal.SystemMintedID(origin),
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
// its prefix. A row another writer stored names its door, not anybody's words.
func reasonFromSource(source string) *string {
	words, ok := strings.CutPrefix(source, seatSourcePrefix+": ")
	if !ok || strings.TrimSpace(words) == "" {
		return nil
	}
	return &words
}
