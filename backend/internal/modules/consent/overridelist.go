// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

type listedOverride struct {
	value   crmcontracts.StandingOverride
	savedAt time.Time
}

func (s *Store) ListContactOverrides(ctx context.Context, contactID ids.ContactID, limit int, cursor string) ([]crmcontracts.StandingOverride, storekit.Page, error) {
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	sub, err := consentSubject(RecordInput{ContactID: contactID})
	if err != nil {
		return nil, storekit.Page{}, err
	}
	pre, err := storekit.BuildListPrelude(ctx, "", nil, nil, nil, &limit, &cursor, nil)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	var entries []listedOverride
	var page storekit.Page
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		subjectID, admitted, err := readableOverrideContact(ctx, tx, sub)
		if err != nil {
			return err
		}
		table := overrideListTable(pre, subjectID)
		scan := func(rows pgx.Rows, _ []fieldcatalog.Column, _ *storekit.ListSort) ([]listedOverride, []*string, error) {
			items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (listedOverride, error) {
				return scanListedOverride(row, ctx, admitted)
			})
			return items, make([]*string, len(items)), err
		}
		entries, page, err = storekit.RunListPageTx(ctx, tx, pre, table,
			"id, category, reason, decided_by_level, created_at, source_recorded_at, legal_entity_id, sending_entity_name, recorded_by_name",
			nil, pre.Where(), scan, func(row listedOverride) (time.Time, ids.UUID) {
				return row.savedAt, ids.UUID(row.value.OverrideId)
			})
		return err
	})
	out := make([]crmcontracts.StandingOverride, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.value)
	}
	return out, page, err
}

func readableOverrideContact(ctx context.Context, tx pgx.Tx, sub subject) (ids.UUID, bool, error) {
	if err := auth.EnsureReadable(ctx, tx, "contact", sub.id); err != nil {
		return ids.UUID{}, false, err
	}
	target, err := survivingSubject(ctx, tx, sub.id)
	if err != nil {
		return ids.UUID{}, false, err
	}
	admitted, err := mayRetractSubject(ctx, tx, sub, requireUpdateOnSubject)
	if err != nil {
		return ids.UUID{}, false, err
	}
	if target != sub.id {
		if err := auth.EnsureReadable(ctx, tx, "contact", target); err != nil {
			return ids.UUID{}, false, err
		}
		sub.id = target
		may, err := mayRetractSubject(ctx, tx, sub, requireUpdateOnSubject)
		if err != nil {
			return ids.UUID{}, false, err
		}
		admitted = admitted && may
	}
	return target, admitted, nil
}

func overrideListTable(pre *storekit.ListPrelude, subjectID ids.UUID) string {
	contact := pre.Arg(subjectID)
	human := pre.Arg(principal.HumanIDPrefix)
	agent := pre.Arg(passportAgentPrefix)
	return storekit.SQLf(`(SELECT o.id, o.category, o.reason, o.decided_by_level,
	        o.recorded_at AS created_at, origin.recorded_at AS source_recorded_at,
	        o.legal_entity_id, le.display_name AS sending_entity_name, u.display_name AS recorded_by_name
	    FROM communication_override o
	    LEFT JOIN LATERAL `+carriedRootSQL("communication_override", "o", "captured_by", "recorded_at")+` origin ON true
	    LEFT JOIN legal_entity le ON le.id = o.legal_entity_id
	    LEFT JOIN passport p ON origin.captured_by = $%d || p.id::text
	    LEFT JOIN app_user u ON origin.captured_by = $%d || u.id::text OR u.id = p.on_behalf_of
	   WHERE o.contact_id = $%d AND o.revoked_at IS NULL) standing`, agent, human, contact)
}

func scanListedOverride(row pgx.CollectableRow, ctx context.Context, admitted bool) (listedOverride, error) {
	var out listedOverride
	var id ids.UUID
	var entity *ids.UUID
	if err := row.Scan(&id, &out.value.Category, &out.value.Reason, &out.value.Authority,
		&out.savedAt, &out.value.RecordedAt, &entity, &out.value.SendingEntityName, &out.value.RecordedByName); err != nil {
		return out, fmt.Errorf("consent: reading a standing exception: %w", err)
	}
	out.value.OverrideId = openapi_types.UUID(id)
	if entity != nil {
		value := openapi_types.UUID(*entity)
		out.value.SendingEntityId = &value
	}
	out.value.MayRevoke = admitted && mayRevokeOverride(ctx, authorityOf(ctx), commsauthz.AuthorityLevel(out.value.Authority))
	return out, nil
}
