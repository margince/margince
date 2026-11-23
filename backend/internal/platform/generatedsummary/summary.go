// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package generatedsummary

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Kind names which summary of a record a row holds.
type Kind string

// The kinds mirror the CHECK on generated_summary.kind.
const (
	KindCompanyBrief     Kind = "company_brief"
	KindCompanyDossier   Kind = "company_dossier"
	KindCompanyGrowthFit Kind = "company_growth_fit"
	KindContactBrief     Kind = "contact_brief"
	KindDealStatusCard   Kind = "deal_status_card"
)

// Subject is one kind of summary about one record.
type Subject struct {
	kind   Kind
	record ids.UUID
}

// CompanyBrief is the account brief of a company.
func CompanyBrief(id ids.CompanyID) Subject {
	return Subject{kind: KindCompanyBrief, record: id.UUID}
}

// CompanyDossier is the dossier of a company.
func CompanyDossier(id ids.CompanyID) Subject {
	return Subject{kind: KindCompanyDossier, record: id.UUID}
}

// CompanyGrowthFit is the growth reading of a company.
func CompanyGrowthFit(id ids.CompanyID) Subject {
	return Subject{kind: KindCompanyGrowthFit, record: id.UUID}
}

// ContactBrief is the relationship brief of a contact.
func ContactBrief(id ids.ContactID) Subject {
	return Subject{kind: KindContactBrief, record: id.UUID}
}

// DealStatusCard is the status card of a deal.
func DealStatusCard(id ids.DealID) Subject {
	return Subject{kind: KindDealStatusCard, record: id.UUID}
}

// Entry is one stored summary and what its reader needs to judge it stale.
type Entry struct {
	Fingerprint string
	GeneratedAt time.Time
	GeneratedBy string
	Payload     []byte
}

// recordKeys puts the records in the key column their kind is held in. The
// other two stay nil, which the table's own CHECK demands of a row.
func recordKeys(kind Kind, records []ids.UUID) (company, contact, deal []ids.UUID) {
	switch kind {
	case KindCompanyBrief, KindCompanyDossier, KindCompanyGrowthFit:
		return records, nil, nil
	case KindContactBrief:
		return nil, records, nil
	case KindDealStatusCard:
		return nil, nil, records
	}
	return nil, nil, nil
}

// Read answers the summary stored for this reader. The reader is part of the
// key because a summary is written from what that reader may see.
func Read(ctx context.Context, tx pgx.Tx, reader ids.UserID, subject Subject) (Entry, bool, error) {
	found, err := ReadEach(ctx, tx, reader, subject.kind, []ids.UUID{subject.record})
	entry, stored := found[subject.record]
	return entry, stored, err
}

// ReadEach answers this reader's summaries of one kind, by the record each is
// about. A record with no stored summary is absent from the answer.
func ReadEach(
	ctx context.Context, tx pgx.Tx, reader ids.UserID, kind Kind, records []ids.UUID,
) (map[ids.UUID]Entry, error) {
	company, contact, deal := recordKeys(kind, records)
	rows, err := tx.Query(ctx, `
		SELECT coalesce(company_id, contact_id, deal_id),
		       fingerprint, generated_at, generated_by, payload
		  FROM generated_summary
		 WHERE user_id = $1 AND kind = $2
		   AND (company_id = ANY($3) OR contact_id = ANY($4) OR deal_id = ANY($5))`,
		reader, kind, company, contact, deal)
	if err != nil {
		return nil, fmt.Errorf("read the generated summaries: %w", err)
	}
	defer rows.Close()
	found := make(map[ids.UUID]Entry, len(records))
	for rows.Next() {
		var record ids.UUID
		var entry Entry
		if err := rows.Scan(&record, &entry.Fingerprint, &entry.GeneratedAt,
			&entry.GeneratedBy, &entry.Payload); err != nil {
			return nil, fmt.Errorf("read the generated summaries: %w", err)
		}
		found[record] = entry
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the generated summaries: %w", err)
	}
	return found, nil
}

// Save replaces the summary stored for this reader.
func Save(ctx context.Context, tx pgx.Tx, reader ids.UserID, subject Subject, written Entry) error {
	company, contact, deal := recordKeys(subject.kind, []ids.UUID{subject.record})
	arguments := []any{
		reader, subject.kind, onlyKey(company), onlyKey(contact), onlyKey(deal),
		written.Fingerprint, written.GeneratedAt, written.GeneratedBy, written.Payload,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO generated_summary (user_id, kind, company_id, contact_id, deal_id,
		                               fingerprint, generated_at, generated_by, payload)
		VALUES (`+database.Placeholders(arguments)+`)
		ON CONFLICT (user_id, kind, company_id, contact_id, deal_id) DO UPDATE
		SET fingerprint = EXCLUDED.fingerprint,
		    generated_at = EXCLUDED.generated_at,
		    generated_by = EXCLUDED.generated_by,
		    payload = EXCLUDED.payload`, arguments...); err != nil {
		return fmt.Errorf("store the generated summary: %w", err)
	}
	return nil
}

// onlyKey is the one record of a key column, or nil for a column left empty.
func onlyKey(records []ids.UUID) *ids.UUID {
	if len(records) == 0 {
		return nil
	}
	return &records[0]
}

// Load is Read in a transaction of its own.
func Load(ctx context.Context, pool *pgxpool.Pool, reader ids.UserID, subject Subject) (Entry, bool, error) {
	var entry Entry
	var stored bool
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		entry, stored, err = Read(ctx, tx, reader, subject)
		return err
	})
	return entry, stored, err
}

// Store is Save in a transaction of its own.
func Store(ctx context.Context, pool *pgxpool.Pool, reader ids.UserID, subject Subject, written Entry) error {
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return Save(ctx, tx, reader, subject, written)
	})
}
