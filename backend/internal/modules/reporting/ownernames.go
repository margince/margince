// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const ownerGroupPrefix = "owner:"

// distinguishOwnerNames adds the email to an owner's name wherever two
// colleagues in one chart share that name, so two rows never read as one owner.
func distinguishOwnerNames(ctx context.Context, tx pgx.Tx, charts []crmcontracts.ReportingChart) error {
	shared := sharedOwnerNames(charts)
	if len(shared) == 0 {
		return nil
	}
	emails, err := ownerEmails(ctx, tx, shared)
	if err != nil {
		return err
	}
	labelSharedOwners(charts, emails)
	return nil
}

// sharedOwnerNames lists the owners whose name another owner in the same chart
// also carries.
func sharedOwnerNames(charts []crmcontracts.ReportingChart) []openapi_types.UUID {
	var shared []openapi_types.UUID
	for _, chart := range charts {
		byName := map[string][]openapi_types.UUID{}
		for _, point := range chart.Points {
			if owner, ok := pointOwner(point); ok {
				byName[point.Label] = append(byName[point.Label], owner)
			}
		}
		for _, owners := range byName {
			if len(owners) > 1 {
				shared = append(shared, owners...)
			}
		}
	}
	return shared
}

func labelSharedOwners(charts []crmcontracts.ReportingChart, emails map[openapi_types.UUID]string) {
	for _, chart := range charts {
		for index := range chart.Points {
			point := &chart.Points[index]
			owner, ok := pointOwner(*point)
			if email := emails[owner]; ok && email != "" {
				point.Label += " · " + email
			}
		}
	}
}

// pointOwner is the colleague a point counts, when its records are one owner's.
func pointOwner(point crmcontracts.ReportingPoint) (openapi_types.UUID, bool) {
	if point.Evidence == nil || point.Evidence.GroupKey == nil {
		return openapi_types.UUID{}, false
	}
	key, ok := strings.CutPrefix(*point.Evidence.GroupKey, ownerGroupPrefix)
	if !ok {
		return openapi_types.UUID{}, false
	}
	parsed, err := ids.Parse(key)
	if err != nil || parsed.IsZero() {
		return openapi_types.UUID{}, false
	}
	return openapi_types.UUID(parsed), true
}

func ownerEmails(ctx context.Context, tx pgx.Tx, owners []openapi_types.UUID) (map[openapi_types.UUID]string, error) {
	var b bindings
	query := "SELECT id,email FROM app_user WHERE id=ANY(" + b.add(owners) + ")"
	rows, err := tx.Query(ctx, query, b.values...)
	if err != nil {
		return nil, err
	}
	type seat struct {
		id    openapi_types.UUID
		email string
	}
	seats, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (seat, error) {
		var s seat
		err := row.Scan(&s.id, &s.email)
		return s, err
	})
	if err != nil {
		return nil, err
	}
	emails := make(map[openapi_types.UUID]string, len(seats))
	for _, s := range seats {
		emails[s.id] = s.email
	}
	return emails, nil
}
