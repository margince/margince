// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A record's owner, named in the file by the seat's email. Import never
// creates a seat: an address no seat holds leaves the importer as the owner,
// and the report says so.

import (
	"cmp"
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const csvTargetOwner = "owner_email"

// seatsByEmail answers the seat each address belongs to, keyed by the folded
// address, in one query. An address no seat holds is absent.
func seatsByEmail(ctx context.Context, tx pgx.Tx, emails []string) (map[string]ids.UUID, error) {
	folded := make([]string, 0, len(emails))
	domains := make([]string, 0, len(emails))
	for _, email := range emails {
		folded = append(folded, foldedEmail(email))
		domains = append(domains, owndomain.OfAddress(email))
	}
	rows, err := tx.Query(ctx, `SELECT u.id, inp.address
		  FROM unnest($1::text[], $2::text[]) AS inp(address, domain_folded)
		  JOIN app_user u ON `+storekit.SeatLoginIsSQL("u", "inp.address", "inp.domain_folded"), folded, domains)
	if err != nil {
		return nil, fmt.Errorf("import: finding the seats these emails belong to: %w", err)
	}
	defer rows.Close()
	seats := make(map[string]ids.UUID, len(folded))
	for rows.Next() {
		var id ids.UUID
		var email string
		if err := rows.Scan(&id, &email); err != nil {
			return nil, fmt.Errorf("import: reading a seat: %w", err)
		}
		seats[email] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("import: finding the seats these emails belong to: %w", err)
	}
	return seats, nil
}

func foldedEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ownerResolver holds the seat of every owner address a run names, read once.
type ownerResolver struct {
	seats map[string]ids.UUID
	// refused are addresses whose seat the importer may not name; the test run
	// reports each once, and its rows keep the importer as owner.
	refused map[string]bool
}

// refuse drops an address's seat, so no row names it as owner.
func (o *ownerResolver) refuse(email string) {
	delete(o.seats, email)
	if o.refused == nil {
		o.refused = map[string]bool{}
	}
	o.refused[email] = true
}

// newOwnerResolver resolves every distinct owner address of the run.
func newOwnerResolver(ctx context.Context, pool *pgxpool.Pool, emails []string) (*ownerResolver, error) {
	o := &ownerResolver{seats: map[string]ids.UUID{}}
	if len(emails) == 0 {
		return o, nil
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		o.seats, err = seatsByEmail(ctx, tx, emails)
		return err
	})
	return o, err
}

// ownerFor is the seat an owner cell names. An address no seat holds answers
// nil, so the store makes the importer the owner, and the warning saying so. A
// refused address answers nil with no warning: the run's own names it.
func (o *ownerResolver) ownerFor(raw string) (*ids.UserID, *migration.SkippedRow) {
	if o.refused[foldedEmail(raw)] {
		return nil, nil
	}
	seat, known := o.seats[foldedEmail(raw)]
	if !known {
		warning := migration.SkippedRow{Kind: migration.ProblemUnknownOwner, Value: strings.TrimSpace(raw)}.Because(unknownOwnerWhy)
		return nil, &warning
	}
	owner := ids.From[ids.UserKind](seat)
	return &owner, nil
}

// ownerOf resolves a row's owner cell, with the warning placed on the row's
// own line and column.
func (w *csvWriters) ownerOf(row migration.Row) (*ids.UserID, *migration.SkippedRow) {
	raw := strings.TrimSpace(textFields(row.Fields)[csvTargetOwner])
	if raw == "" {
		return nil, nil
	}
	owner, warning := w.owners.ownerFor(raw)
	if warning != nil {
		warning.ExternalID, warning.Line, warning.Column = row.ExternalID, row.Line, w.headerFor(csvTargetOwner)
	}
	return owner, warning
}

// landingCells is checkedCells with the row's owner: a known seat rides in
// typed. An unknown address or an unsplit line 1 is the warning the row lands
// with.
func (w *csvWriters) landingCells(row migration.Row) (map[string]any, *migration.SkippedRow, *migration.SkippedRow) {
	typed, invalid := w.checkedCells(row)
	if invalid != nil {
		return nil, invalid, nil
	}
	owner, warning := w.ownerOf(row)
	if owner != nil {
		if typed == nil {
			typed = map[string]any{}
		}
		typed[csvTargetOwner] = owner.UUID
	}
	return typed, nil, cmp.Or(warning, w.unsplitAddressWarning(row))
}

// ownerFrom is the seat a create or an update names, nil when the file named
// none this run knows.
func ownerFrom(fields map[string]string, typed map[string]any) *ids.UserID {
	if _, given := fields[csvTargetOwner]; !given {
		return nil
	}
	seat, known := typed[csvTargetOwner].(ids.UUID)
	if !known {
		return nil
	}
	owner := ids.From[ids.UserKind](seat)
	return &owner
}

// collapseUnknownOwners folds the unknown-owner warnings of one file that name
// one address into a single issue. It sits on the first row that names the
// address and carries how many rows do. Every row still lands with the
// importer as owner.
func collapseUnknownOwners(issues []crmcontracts.ImportRowIssue) []crmcontracts.ImportRowIssue {
	first := map[string]int{}
	counts := map[string]int{}
	// Non-nil: an empty list is `[]` on the wire, never null.
	out := make([]crmcontracts.ImportRowIssue, 0, len(issues))
	for _, issue := range issues {
		if issue.Kind == nil || string(*issue.Kind) != migration.ProblemUnknownOwner {
			out = append(out, issue)
			continue
		}
		key := derefString(issue.File) + "\x00" + foldedEmail(derefString(issue.Value))
		if _, seen := first[key]; !seen {
			first[key] = len(out)
			out = append(out, issue)
		}
		counts[key]++
	}
	for key, at := range first {
		why := migration.WhyCount(migration.ReasonOwnerUnknown, "rows", counts[key])
		was := out[at]
		out[at] = crmcontracts.ImportRowIssue{
			Line: was.Line, File: was.File, Column: was.Column, Kind: was.Kind, Value: was.Value,
			Reason: why.English(), ReasonCode: reasonCodeOrAbsent(why.Code), Params: paramsOrAbsent(why.Params),
		}
	}
	return out
}
