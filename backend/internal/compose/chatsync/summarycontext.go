// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// What a summary request carries besides the talk: the note filed for the
// channel's previous stretch, and the parties on the record. Both are read the
// way every member may read them, because the note is filed where the whole
// workspace reads it. The deal's amount is never among them.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// maxContactFacts bounds the contact names one request names.
const maxContactFacts = 8

// summaryContext is the extra a summary request gets.
type summaryContext struct {
	facts   []string
	earlier string
}

// readSummaryContext reads the company and contact names of a deal, and the
// note the channel filed for talk that began before the given time. A project
// has no company or contacts here, and a channel's first stretch has no note.
func readSummaryContext(ctx context.Context, pool *pgxpool.Pool, link chatlinks.Link, record chatRecord, before time.Time) (summaryContext, error) {
	var out summaryContext
	if record.ownerName != "" {
		out.facts = append(out.facts, "Owner: "+record.ownerName)
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if link.DealID != nil {
			facts, err := dealParties(ctx, tx, *link.DealID)
			if err != nil {
				return err
			}
			out.facts = append(out.facts, facts...)
		}
		note, err := previousNote(ctx, tx, link, before)
		out.earlier = note
		return err
	})
	return out, err
}

// dealParties names the deal's company and its stakeholders. A contact or
// company kept private to its owner, an archived one and a contact under a
// restriction are left out.
func dealParties(ctx context.Context, tx pgx.Tx, dealID ids.UUID) ([]string, error) {
	var facts []string
	var company string
	err := tx.QueryRow(ctx, `SELECT c.display_name FROM deal d JOIN company c ON c.id = d.company_id
		WHERE d.id = $1 AND c.archived_at IS NULL AND c.visibility = 'workspace'`, dealID).Scan(&company)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}
	if company != "" {
		facts = append(facts, "Company: "+company)
	}
	rows, err := tx.Query(ctx, `SELECT p.full_name FROM relationship r JOIN contact p ON p.id = r.contact_id
		WHERE r.deal_id = $1 AND r.kind = 'deal_stakeholder' AND r.archived_at IS NULL
		  AND p.archived_at IS NULL AND p.visibility = 'workspace' AND NOT `+auth.ContactRestrictedSQL("p.id")+`
		ORDER BY r.is_current_primary DESC, p.full_name, p.id LIMIT $2`, dealID, maxContactFacts)
	if err != nil {
		return nil, err
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	if len(names) > 0 {
		facts = append(facts, "Contacts: "+strings.Join(names, ", "))
	}
	return facts, nil
}

// previousNote is the latest summary note of the link's channel, filed on the
// link's record, for talk that began before the given time. It reads as empty
// when that note is private, archived or restricted.
func previousNote(ctx context.Context, tx pgx.Tx, link chatlinks.Link, before time.Time) (string, error) {
	record, column := link.DealID, "deal_id"
	if record == nil {
		record, column = link.ProjectID, "project_id"
	}
	if record == nil {
		return "", nil
	}
	note, err := sharedRow(ctx, tx, `a.kind = 'note' AND a.source_system = $1
		AND left(a.source_id, length($2::text) + 1) = $2::text || ':' AND a.occurred_at < $3 -- occurred_at raw: keyset bound before the episode's first message
		AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.`+column+` = $4)`,
		provenance.ChatSummarySource, link.ChannelSlug, before, *record)
	if err != nil || !note.ok {
		return "", err
	}
	return strings.TrimSpace(note.subject + "\n" + note.body), nil
}
