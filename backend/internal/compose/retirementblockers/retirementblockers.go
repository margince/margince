// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package retirementblockers finds what keeps a member from being retired: the
// live rows that still name them, grouped by the blocker each one raises. Each
// blocker lists the records behind it that the reader may open.
//
// A record is listed under the read rule of the store that owns it. A table with
// no rule here is counted and never named, so a personal row stays its owner's.
package retirementblockers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListedPerBlocker caps the records one blocker names, as the contract does.
const ListedPerBlocker = 25

// Reference is one column whose live rows keep the member they name from
// retiring. Table, Column and Live are compile-time SQL from the reference
// policy, never request input.
type Reference struct {
	Table, Column, Live string
	Blocker             crmcontracts.MemberRetirementBlocker
}

// Found holds the blockers the references raise, sorted, and the records
// behind each one, in the same order.
type Found struct {
	Blockers []crmcontracts.MemberRetirementBlocker
	Records  []crmcontracts.MemberRetirementBlockingRecords
}

type blockingRow struct {
	id       ids.UUID
	name     string
	pageType string
	page     *ids.UUID
	visible  bool
}

// held is what one blocker holds: every blocking row, and those rows of a
// table the preview may name.
type held struct {
	total int
	named []blockingRow
}

// Find reads the member's live references. origin is the app origin record
// links are built on; empty means the records carry no link.
func Find(ctx context.Context, tx pgx.Tx, member ids.UUID, refs []Reference, origin string) (Found, error) {
	holds := map[crmcontracts.MemberRetirementBlocker]*held{}
	for _, ref := range refs {
		total, named, err := readReference(ctx, tx, member, ref)
		if err != nil {
			return Found{}, err
		}
		if total == 0 {
			continue
		}
		if holds[ref.Blocker] == nil {
			holds[ref.Blocker] = &held{}
		}
		holds[ref.Blocker].total += total
		holds[ref.Blocker].named = append(holds[ref.Blocker].named, named...)
	}
	out := Found{Blockers: []crmcontracts.MemberRetirementBlocker{}, Records: []crmcontracts.MemberRetirementBlockingRecords{}}
	for _, blocker := range slices.Sorted(maps.Keys(holds)) {
		out.Blockers = append(out.Blockers, blocker)
		out.Records = append(out.Records, describe(blocker, *holds[blocker], origin))
	}
	return out, nil
}

func describe(blocker crmcontracts.MemberRetirementBlocker, h held, origin string) crmcontracts.MemberRetirementBlockingRecords {
	entry := crmcontracts.MemberRetirementBlockingRecords{Blocker: blocker, Total: h.total, Records: []crmcontracts.MemberRetirementBlockingRecord{}}
	slices.SortFunc(h.named, func(a, b blockingRow) int {
		return cmp.Or(cmp.Compare(a.name, b.name), cmp.Compare(a.id.String(), b.id.String()))
	})
	for _, row := range h.named {
		if !row.visible {
			continue
		}
		entry.Visible++
		if len(entry.Records) < ListedPerBlocker {
			entry.Records = append(entry.Records, listed(row, origin))
		}
	}
	return entry
}

func listed(row blockingRow, origin string) crmcontracts.MemberRetirementBlockingRecord {
	record := crmcontracts.MemberRetirementBlockingRecord{Id: openapi_types.UUID(row.id), Name: row.name}
	if row.page == nil || origin == "" {
		return record
	}
	if fragment := mailcopy.RecordFragment(row.pageType, row.page.String()); fragment != "" {
		link := mailcopy.Destination(origin, fragment)
		record.WebUrl = &link
	}
	return record
}

// readReference counts the rows that name the member. A table the preview may
// name also hands over each row, marked by whether the reader may open it.
func readReference(ctx context.Context, tx pgx.Tx, member ids.UUID, ref Reference) (int, []blockingRow, error) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	from := fmt.Sprintf(`FROM %s b WHERE b.%s = $%d AND (%s)`,
		pgx.Identifier{ref.Table}.Sanitize(), pgx.Identifier{ref.Column}.Sanitize(), arg(member), ref.Live)
	reader, named := readers[ref.Table+"."+ref.Column]
	if !named {
		var total int
		if err := tx.QueryRow(ctx, "SELECT count(*) "+from, args...).Scan(&total); err != nil {
			return 0, nil, fmt.Errorf("count the %s rows that name the member: %w", ref.Table, err)
		}
		return total, nil, nil
	}
	visible, err := reader.visibleSQL(ctx, arg)
	if err != nil {
		return 0, nil, err
	}
	found, err := tx.Query(ctx, fmt.Sprintf(`SELECT b.id, %s, %s, %s `, reader.name, cmp.Or(reader.pageID, "NULL::uuid"), visible)+from, args...)
	if err != nil {
		return 0, nil, fmt.Errorf("read the %s rows that name the member: %w", ref.Table, err)
	}
	rows, err := pgx.CollectRows(found, func(row pgx.CollectableRow) (blockingRow, error) {
		r := blockingRow{pageType: reader.pageType}
		err := row.Scan(&r.id, &r.name, &r.page, &r.visible)
		return r, err
	})
	return len(rows), rows, err
}

// visibleSQL renders the owning store's read rule over the row aliased b. A
// missing object grant hides every row, and the probe records no requirement:
// the preview answers without the records rather than refusing.
func (r reader) visibleSQL(ctx context.Context, arg func(any) int) (string, error) {
	readCtx := ctx
	for _, object := range r.objects {
		next, err := auth.Unrecorded(readCtx, object, principal.ActionRead)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "FALSE", nil
		}
		if err != nil {
			return "", err
		}
		readCtx = next
	}
	if r.rows == nil {
		return sqlTrue, nil
	}
	return r.rows(readCtx, arg)
}
