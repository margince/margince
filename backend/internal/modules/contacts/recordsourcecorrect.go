// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// unlistedPartySource selects the rows whose source predates the fixed list.
var unlistedPartySource = `NOT (` + provenance.ListedRecordSourceSQL("source") + `)`

// partySourceSQL is one table's page and update statements for the source
// correction. The writer an older word named moves into source_system.
type partySourceSQL struct{ page, update string }

var contactSourceSQL = partySourceSQL{
	page: `SELECT id, source, party_source_channel(source, source_system) FROM contact
		  WHERE id > $1 AND ` + unlistedPartySource + ` ORDER BY id LIMIT $2 FOR UPDATE`,
	update: `UPDATE contact r SET source = moved.source,
		        source_system = COALESCE(r.source_system, party_source_producer(r.source))
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
}

var companySourceSQL = partySourceSQL{
	page: `SELECT id, source, party_source_channel(source, source_system) FROM company
		  WHERE id > $1 AND ` + unlistedPartySource + ` ORDER BY id LIMIT $2 FOR UPDATE`,
	update: `UPDATE company r SET source = moved.source,
		        source_system = COALESCE(r.source_system, party_source_producer(r.source))
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
}

// leadSourceSQL reads a lead's older word as a contact's, since a lead is
// promoted into one. A lead's source_system is its replay key, so it stays.
var leadSourceSQL = partySourceSQL{
	page: `SELECT id, source, party_source_channel(source, source_system) FROM lead
		  WHERE id > $1 AND ` + unlistedPartySource + ` ORDER BY id LIMIT $2 FOR UPDATE`,
	update: `UPDATE lead r SET source = moved.source
		   FROM unnest($1::uuid[], $2::text[]) AS moved(id, source) WHERE r.id = moved.id`,
}

// CorrectContactSourcesTx moves one page of contacts whose source predates the
// fixed list. A row on the list is left out, so a replay changes nothing. Each
// contact announces it as contact.updated, as an edit of the field would.
func CorrectContactSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, entityContact, contactSourceSQL.page, contactSourceSQL.update, after, limit, runID)
	if err != nil {
		return nil, err
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		if err := storekit.EmitEvent(ctx, tx, row.AuditID, row.ID, crmcontracts.PublicEventContactUpdated{
			ChangedFields: map[string]any{storekit.RecordSourceField: row.Source},
		}); err != nil {
			return nil, fmt.Errorf("emit contact.updated: %w", err)
		}
		moved[i] = row.ID
	}
	return moved, nil
}

// CorrectCompanySourcesTx is CorrectContactSourcesTx for companies.
func CorrectCompanySourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, entityCompany, companySourceSQL.page, companySourceSQL.update, after, limit, runID)
	if err != nil {
		return nil, err
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		if err := storekit.EmitEvent(ctx, tx, row.AuditID, row.ID, crmcontracts.PublicEventCompanyUpdated{
			ChangedFields: map[string]any{storekit.RecordSourceField: row.Source},
		}); err != nil {
			return nil, fmt.Errorf("emit company.updated: %w", err)
		}
		moved[i] = row.ID
	}
	return moved, nil
}

// CorrectLeadSourcesTx is CorrectContactSourcesTx for leads.
func CorrectLeadSourcesTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int, runID ids.UUID) ([]ids.UUID, error) {
	corrected, err := storekit.CorrectRecordSources(ctx, tx, entityLead, leadSourceSQL.page, leadSourceSQL.update, after, limit, runID)
	if err != nil {
		return nil, err
	}
	moved := make([]ids.UUID, len(corrected))
	for i, row := range corrected {
		if err := storekit.EmitEvent(ctx, tx, row.AuditID, row.ID, crmcontracts.PublicEventLeadUpdated{
			ChangedFields: map[string]any{storekit.RecordSourceField: row.Source},
		}); err != nil {
			return nil, fmt.Errorf("emit lead.updated: %w", err)
		}
		moved[i] = row.ID
	}
	return moved, nil
}

// countUnlisted runs a count of the rows a correction would still move,
// reading at most maximum+1.
func countUnlisted(ctx context.Context, q datatask.Querier, table, sql string, maximum int64) (int64, error) {
	var count int64
	if err := q.QueryRow(ctx, sql, maximum+1).Scan(&count); err != nil {
		return 0, fmt.Errorf("contacts: counting %s rows with an unlisted source: %w", table, err)
	}
	return count, nil
}

// CountUnlistedContactSources counts the contacts CorrectContactSourcesTx would still move.
func CountUnlistedContactSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	return countUnlisted(ctx, q, entityContact,
		`SELECT count(*) FROM (SELECT 1 FROM contact WHERE `+unlistedPartySource+` LIMIT $1) unlisted`, maximum)
}

// CountUnlistedCompanySources counts the companies CorrectCompanySourcesTx would still move.
func CountUnlistedCompanySources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	return countUnlisted(ctx, q, entityCompany,
		`SELECT count(*) FROM (SELECT 1 FROM company WHERE `+unlistedPartySource+` LIMIT $1) unlisted`, maximum)
}

// CountUnlistedLeadSources counts the leads CorrectLeadSourcesTx would still move.
func CountUnlistedLeadSources(ctx context.Context, q datatask.Querier, maximum int64) (int64, error) {
	return countUnlisted(ctx, q, entityLead,
		`SELECT count(*) FROM (SELECT 1 FROM lead WHERE `+unlistedPartySource+` LIMIT $1) unlisted`, maximum)
}
