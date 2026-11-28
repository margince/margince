// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// What a summary request carries besides the talk. It holds the note filed for
// the channel's previous stretch and the parties on the record. It also holds
// the seat addresses and own domains that tell a colleague. Names are judged
// against the members who may read the summary note. The activity gate limits
// that note to holders of read on the link's deal or project. The deal's amount
// is never among them.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/dealseat"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// maxContactFacts bounds the contact names one request names.
const maxContactFacts = 8

// summaryContext is the extra a summary request gets.
type summaryContext struct {
	facts   []string
	earlier string
	place   placing
}

// readSummaryContext reads the owner, and for a deal its company and contact
// names. It also reads the note the channel filed for talk that ended before
// the given time. A project has no company or contacts here.
func readSummaryContext(ctx context.Context, pool *pgxpool.Pool, link chatlinks.Link, record chatRecord, before time.Time) (summaryContext, error) {
	var out summaryContext
	// One snapshot, so every group of readers judges the same company and edges.
	err := database.WithSnapshot(ctx, pool, func(ctx context.Context) error {
		return database.WithTx(ctx, pool, func(tx pgx.Tx) error {
			owner, err := liveOwner(ctx, tx, record)
			if err != nil {
				return err
			}
			if owner != "" {
				out.facts = append(out.facts, "Owner: "+owner)
			}
			readers, err := noteReaders(ctx, tx, link)
			if err != nil {
				return err
			}
			if link.DealID != nil {
				facts, err := dealParties(ctx, tx, *link.DealID, readers)
				if err != nil {
					return err
				}
				out.facts = append(out.facts, facts...)
			}
			if out.earlier, err = previousNote(ctx, tx, link, before); err != nil {
				return err
			}
			if out.place.seats, err = storekit.WorkspaceSeatAddresses(ctx, tx); err != nil {
				return err
			}
			out.place.own, err = capture.TrustedColleagueDomainsTx(ctx, tx)
			return err
		})
	})
	return out, err
}

// liveOwner is the record owner's name, empty when the owner has left.
func liveOwner(ctx context.Context, tx pgx.Tx, record chatRecord) (string, error) {
	table := record.kind
	if record.kind == chatlinks.RecordDeal {
		table = "deal"
	}
	var name string
	err := tx.QueryRow(ctx, `SELECT coalesce(u.display_name, '') FROM `+table+` x JOIN app_user u ON u.id = x.owner_id
		WHERE x.id = $1 AND `+workforce.LiveMemberSQL("u"), record.id).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return name, err
}

// noteReaders lists the live human members who may read the summary note. The
// activity gate admits those holding read on the link's deal or project.
func noteReaders(ctx context.Context, tx pgx.Tx, link chatlinks.Link) ([]principal.Principal, error) {
	rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE `+workforce.LiveHumanSQL("u")+` ORDER BY u.id`)
	if err != nil {
		return nil, fmt.Errorf("chat summary: reading the members: %w", err)
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	byID, err := identity.MemberPrincipalsTx(ctx, tx, users)
	if err != nil {
		return nil, err
	}
	readers := make([]principal.Principal, 0, len(users))
	for _, id := range users {
		if p, live := byID[id]; live && holdsReadOnLink(principal.WithActor(ctx, p), link) {
			readers = append(readers, p)
		}
	}
	return readers, nil
}

// holdsReadOnLink is whether the actor may read the link's deal or project.
func holdsReadOnLink(ctx context.Context, link chatlinks.Link) bool {
	return link.CompanyID != nil && auth.Allows(ctx, chatlinks.RecordCompany, principal.ActionRead) ||
		link.DealID != nil && auth.Allows(ctx, chatlinks.RecordDeal, principal.ActionRead) ||
		link.ProjectID != nil && auth.Allows(ctx, chatlinks.RecordProject, principal.ActionRead)
}

// dealParties names the deal's company and its current stakeholders. Each name
// needs every reader's object read and row scope, and a stakeholder also the
// edge read. With no reader, or a refused one, the section is left out.
func dealParties(ctx context.Context, tx pgx.Tx, dealID ids.UUID, readers []principal.Principal) ([]string, error) {
	if len(readers) == 0 {
		return nil, nil
	}
	var facts []string
	company, err := companyName(ctx, tx, dealID, readers)
	if err != nil {
		return nil, err
	}
	if company != "" {
		facts = append(facts, "Company: "+company)
	}
	names, err := stakeholderNames(ctx, tx, dealID, readers)
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		facts = append(facts, "Contacts: "+strings.Join(names, ", "))
	}
	return facts, nil
}

// companyName is the deal's company when every reader may read it.
func companyName(ctx context.Context, tx pgx.Tx, dealID ids.UUID, readers []principal.Principal) (string, error) {
	if err := auth.RequireReadForReaders(ctx, "company", readers...); err != nil {
		return "", withheld(err)
	}
	var name string
	for group := range auth.ReaderGroups(readers) {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		deal := arg(dealID)
		scope, err := auth.ScopeClauseForReaders(ctx, "company", "c", arg, group...)
		if err != nil {
			return "", withheld(err)
		}
		err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT c.display_name FROM deal d JOIN company c ON c.id = d.company_id
			WHERE d.id = $%d AND c.archived_at IS NULL AND c.visibility = 'workspace'%s`, deal, andClause(scope)), args...).Scan(&name)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
	}
	return name, nil
}

// stakeholder is one current stakeholder a group of readers may all read.
type stakeholder struct {
	id   ids.UUID
	name string
}

// stakeholderNames are the deal's current stakeholders every reader may read,
// in the order the first group of readers sees them.
func stakeholderNames(ctx context.Context, tx pgx.Tx, dealID ids.UUID, readers []principal.Principal) ([]string, error) {
	var shared []stakeholder
	first := true
	for group := range auth.ReaderGroups(readers) {
		seen, err := stakeholdersReadBy(ctx, tx, dealID, group)
		if err != nil {
			return nil, err
		}
		if first {
			shared, first = distinctStakeholders(seen), false
		} else {
			shared = seenByAll(shared, seen)
		}
		if len(shared) == 0 {
			return nil, nil
		}
	}
	names := make([]string, 0, min(len(shared), maxContactFacts))
	for _, s := range shared[:min(len(shared), maxContactFacts)] {
		names = append(names, s.name)
	}
	return names, nil
}

// distinctStakeholders keeps each contact once, at its first edge.
func distinctStakeholders(rows []stakeholder) []stakeholder {
	kept := map[ids.UUID]bool{}
	return slices.DeleteFunc(rows, func(s stakeholder) bool {
		if kept[s.id] {
			return true
		}
		kept[s.id] = true
		return false
	})
}

// seenByAll keeps the stakeholders a later group of readers saw too.
func seenByAll(shared, seen []stakeholder) []stakeholder {
	later := make(map[ids.UUID]bool, len(seen))
	for _, s := range seen {
		later[s.id] = true
	}
	return slices.DeleteFunc(shared, func(s stakeholder) bool { return !later[s.id] })
}

// stakeholdersReadBy are the deal's current stakeholders each of readers may
// read, with the object and row scope of the contact and the edge.
func stakeholdersReadBy(ctx context.Context, tx pgx.Tx, dealID ids.UUID, readers []principal.Principal) ([]stakeholder, error) {
	if err := auth.RequireReadForReaders(ctx, "contact", readers...); err != nil {
		return nil, withheld(err)
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	deal := arg(dealID)
	scopes := []string{}
	contactScope, err := auth.ScopeClauseForReaders(ctx, "contact", "p", arg, readers...)
	if err != nil {
		return nil, withheld(err)
	}
	scopes = append(scopes, contactScope)
	for _, reader := range readers {
		edge, err := auth.EdgeReadScope(principal.WithActor(ctx, reader), "r", arg)
		if err != nil {
			return nil, withheld(err)
		}
		scopes = append(scopes, edge)
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT p.id, p.full_name FROM relationship r JOIN contact p ON p.id = r.contact_id
		WHERE r.deal_id = $%d AND %s
		  AND p.archived_at IS NULL AND p.visibility = 'workspace' AND NOT `+auth.ContactRestrictedSQL("p.id")+`%s
		ORDER BY r.is_current_primary DESC, p.full_name, p.id`, deal, dealseat.CurrentSQL("r"), andClause(strings.Join(nonEmpty(scopes), " AND "))), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (stakeholder, error) {
		var s stakeholder
		return s, row.Scan(&s.id, &s.name)
	})
}

// withheld turns a reader's refusal into "leave the section out" and passes
// every other error on.
func withheld(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
}

func andClause(clause string) string {
	if clause == "" {
		return ""
	}
	return " AND " + clause
}

func nonEmpty(clauses []string) []string {
	out := clauses[:0:0]
	for _, c := range clauses {
		if c != "" {
			out = append(out, "("+c+")")
		}
	}
	return out
}

// previousNote is the latest summary note of the link's channel, filed on the
// link's record, whose talk ended before the given time. Ties on the end go to
// the later channel sequence. It reads as empty when that note is private,
// archived or restricted. Being a chat summary on this record, it has the
// readers the new note has.
func previousNote(ctx context.Context, tx pgx.Tx, link chatlinks.Link, before time.Time) (string, error) {
	record, column := linkRecord(link), "project_id"
	if record.ID.IsZero() {
		return "", nil
	}
	switch record.Type {
	case chatlinks.RecordCompany:
		column = "company_id"
	case chatlinks.RecordDeal:
		column = "deal_id"
	}
	const seq = `split_part(substring(a.source_id from length($2::text) + 2), ':', 1)::bigint`
	note, err := sharedRow(ctx, tx, `a.kind = 'note' AND a.source_system = $1
		AND left(a.source_id, length($2::text) + 1) = $2::text || ':'
		AND (a.raw->>'ended_at')::timestamptz < $3
		AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.`+column+` = $4)`,
		`(a.raw->>'ended_at')::timestamptz DESC, `+seq+` DESC, a.id DESC`,
		provenance.ChatSummarySource, link.ChannelSlug, before, record.ID)
	if err != nil || !note.ok {
		return "", err
	}
	return strings.TrimSpace(note.subject + "\n" + note.body), nil
}
