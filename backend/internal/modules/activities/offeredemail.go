// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
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

// eligibleRowsSQL keeps the live written-about rows. A task the product filed
// has nobody to write to.
var eligibleRowsSQL = `
	SELECT a.id, a.kind
	  FROM activity a
	 WHERE a.id = ANY($1) AND a.kind = ANY($2) AND a.archived_at IS NULL AND activity_thread_available(a.thread_key)
	   AND (a.kind <> '` + string(crmcontracts.ActivityKindTask) + `' OR ` + relstrength.OriginIsEngagementSQL("a") + `)`

// contactCandidatesSQL reads the contacts a row is filed under, with the
// address Write email would use, under the contact scope.
const contactCandidatesSQL = `
	SELECT l.activity_id, p.id, p.full_name,
	       coalesce((SELECT pe.email FROM contact_email pe
	                  WHERE pe.contact_id = p.id AND pe.archived_at IS NULL` + contactaddress.ReachableOrder + `
	                  LIMIT 1), ''),
	       p.archived_at IS NOT NULL
	  FROM activity_link l
	  JOIN contact p ON p.id = l.contact_id
	 WHERE l.activity_id = ANY($1) AND l.contact_id IS NOT NULL %s`

// emailTargets is the record an email about each written-about row goes to:
// pickEmailTarget over what the row is filed under. offers_email, the
// reply-recipient read and the drafts all take their answer from it.
func emailTargets(ctx context.Context, tx pgx.Tx, rows []ids.UUID) (map[ids.UUID]EmailTarget, error) {
	kinds, err := eligibleRows(ctx, tx, rows)
	if err != nil || len(kinds) == 0 {
		return map[ids.UUID]EmailTarget{}, err
	}
	var eligible, tasks []ids.UUID
	for row, kind := range kinds {
		eligible = append(eligible, row)
		if kind == crmcontracts.ActivityKindTask {
			tasks = append(tasks, row)
		}
	}
	contacts, err := linkedCandidates(ctx, tx, "contact", "p", contactCandidatesSQL, eligible)
	if err != nil {
		return nil, err
	}
	leads, err := linkedCandidates(ctx, tx, "lead", "ld", leadCandidatesSQL, tasks)
	if err != nil {
		return nil, err
	}
	companies, err := linkedCandidates(ctx, tx, "company", "co", companyCandidatesSQL, tasks)
	if err != nil {
		return nil, err
	}
	reachable, err := reachableAddresses(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := map[ids.UUID]EmailTarget{}
	for row, kind := range kinds {
		found := emailCandidates{
			contacts:  liveFirst(reachable(contacts[row])),
			leads:     reachable(leads[row]),
			companies: companies[row],
		}
		if target, named := pickEmailTarget(kind, found); named {
			out[row] = target
		}
	}
	return out, nil
}

// liveFirst keeps the live contacts, or the archived ones when none is live.
// A row on one live and one archived contact writes to the live one.
func liveFirst(contacts []emailCandidate) []emailCandidate {
	var live []emailCandidate
	for _, contact := range contacts {
		if !contact.closed {
			live = append(live, contact)
		}
	}
	if len(live) > 0 {
		return live
	}
	return contacts
}

func eligibleRows(ctx context.Context, tx pgx.Tx, rows []ids.UUID) (map[ids.UUID]crmcontracts.ActivityKind, error) {
	out := map[ids.UUID]crmcontracts.ActivityKind{}
	if len(rows) == 0 {
		return out, nil
	}
	kinds := make([]string, 0, len(writtenAboutKinds))
	for _, kind := range writtenAboutKinds {
		kinds = append(kinds, string(kind))
	}
	found, err := tx.Query(ctx, eligibleRowsSQL, rows, kinds)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the rows an email is written about: %w", err)
	}
	defer found.Close()
	for found.Next() {
		var row ids.UUID
		var kind crmcontracts.ActivityKind
		if err := found.Scan(&row, &kind); err != nil {
			return nil, err
		}
		out[row] = kind
	}
	return out, found.Err()
}

// reachableAddresses returns a filter that blanks every address Write email may
// not use: a shared mailbox's, one at our own domains, or a seat's login.
func reachableAddresses(ctx context.Context, tx pgx.Tx) (func([]emailCandidate) []emailCandidate, error) {
	own, err := owndomain.Read(ctx, tx, auth.TrustedOwnDomainsSQL)
	if err != nil {
		return nil, err
	}
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil {
		return nil, err
	}
	return func(candidates []emailCandidate) []emailCandidate {
		out := make([]emailCandidate, len(candidates))
		for i, candidate := range candidates {
			if candidate.address != "" && (mailrole.GreetsNobody(candidate.name, candidate.address) ||
				own.Covers(candidate.address) || seats.IsLogin(candidate.address)) {
				candidate.address = ""
			}
			out[i] = candidate
		}
		return out
	}, nil
}

// WithOffersEmail sets offers_email and email_target on a page of rows. A row whose content is
// withheld offers nothing: the composer cannot open on it.
func WithOffersEmail(ctx context.Context, tx pgx.Tx, page []crmcontracts.Activity) error {
	var asked []ids.UUID
	for i := range page {
		readable := page[i].ContentState != nil && *page[i].ContentState == crmcontracts.ActivityContentStateAvailable
		if readable && slices.Contains(writtenAboutKinds, page[i].Kind) {
			asked = append(asked, ids.UUID(page[i].Id))
		}
	}
	targets, err := emailTargets(ctx, tx, asked)
	if err != nil {
		return err
	}
	for i := range page {
		target, named := targets[ids.UUID(page[i].Id)]
		if !named {
			continue
		}
		page[i].OffersEmail = target.offered() && target.Address != ""
		wire := crmcontracts.ActivityEmailTarget{
			EntityType: target.EntityType,
			EntityId:   openapi_types.UUID(target.EntityID),
		}
		if target.Refusal != "" {
			refusal := target.Refusal
			wire.Refusal = &refusal
		}
		page[i].EmailTarget = &wire
	}
	return nil
}
