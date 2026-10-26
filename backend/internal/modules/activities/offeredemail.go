// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/mailrole"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/contactaddress"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// writtenAboutKinds are the kinds a rep writes about rather than replies to.
var writtenAboutKinds = []crmcontracts.ActivityKind{
	crmcontracts.ActivityKindCall, crmcontracts.ActivityKindMeeting,
	crmcontracts.ActivityKindNote, crmcontracts.ActivityKindTask,
}

// offeredEmailCandidatesSQL keeps a live row with one readable live linked contact
// whose address is not a seat's login. A task the product filed has nobody to write to.
var offeredEmailCandidatesSQL = `
	SELECT c.activity_id, c.full_name, c.address
	  FROM (SELECT l.activity_id, p.full_name,
	               coalesce((SELECT pe.email FROM contact_email pe
	                          WHERE pe.contact_id = p.id AND pe.archived_at IS NULL` + contactaddress.ReachableOrder + `
	                          LIMIT 1), '') AS address,
	               count(*) OVER (PARTITION BY l.activity_id) AS linked
	          FROM activity a
	          JOIN activity_link l ON l.activity_id = a.id AND l.contact_id IS NOT NULL
	          JOIN contact p ON p.id = l.contact_id AND p.archived_at IS NULL
	         WHERE a.id = ANY($1) AND a.kind = ANY($2) AND a.archived_at IS NULL
	           AND (a.kind <> '` + string(crmcontracts.ActivityKindTask) + `' OR ` + relstrength.OriginIsEngagementSQL("a") + `) %s) c
	 WHERE c.linked = 1 AND c.address <> ''
	   AND NOT EXISTS (SELECT 1 FROM app_user seat WHERE lower(seat.email) = lower(c.address))`

// offeredEmails is the address Write email opens with, per written-about row. The
// reply-recipient read and every activity read take their answer from it.
//
// Our own domains are judged by owndomain, the rule a reply's To also walks.
// SQL cannot fold an IDNA domain the way that rule does.
func offeredEmails(ctx context.Context, tx pgx.Tx, rows []ids.UUID) (map[ids.UUID]string, error) {
	out := map[ids.UUID]string{}
	if len(rows) == 0 || !auth.ReadGranted(ctx, "contact") {
		return out, nil
	}
	kinds := make([]string, 0, len(writtenAboutKinds))
	for _, kind := range writtenAboutKinds {
		kinds = append(kinds, string(kind))
	}
	args := []any{rows, kinds}
	scope, err := auth.ScopeClauseFor(ctx, "contact", "p", func(v any) int {
		args = append(args, v)
		return len(args)
	})
	if err != nil {
		return nil, err
	}
	if scope != "" {
		scope = `AND (` + scope + `)`
	}
	found, err := tx.Query(ctx, fmt.Sprintf(offeredEmailCandidatesSQL, scope), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the contact each row is filed on: %w", err)
	}
	defer found.Close()
	for found.Next() {
		var row ids.UUID
		var name, address string
		if err := found.Scan(&row, &name, &address); err != nil {
			return nil, err
		}
		if !mailrole.GreetsNobody(name, address) {
			out[row] = address
		}
	}
	if err := found.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	return withoutColleagues(ctx, tx, out)
}

// withoutColleagues drops the addresses at one of our own domains.
func withoutColleagues(ctx context.Context, tx pgx.Tx, offered map[ids.UUID]string) (map[ids.UUID]string, error) {
	own, err := owndomain.Read(ctx, tx, auth.TrustedOwnDomainsSQL)
	if err != nil {
		return nil, err
	}
	maps.DeleteFunc(offered, func(_ ids.UUID, address string) bool { return own.Covers(address) })
	return offered, nil
}

// WithOffersEmail sets offers_email on a page of rows. A row whose content is
// withheld offers nothing: the composer cannot open on it.
func WithOffersEmail(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	var asked []ids.UUID
	for i := range page {
		readable := page[i].ContentState != nil && *page[i].ContentState == crmcontracts.ActivityContentStateAvailable
		if readable && slices.Contains(writtenAboutKinds, page[i].Kind) {
			asked = append(asked, ids.UUID(page[i].Id))
		}
	}
	offered, err := offeredEmails(ctx, tx, asked)
	if err != nil {
		return err
	}
	for i := range page {
		_, page[i].OffersEmail = offered[ids.UUID(page[i].Id)]
	}
	return nil
}
