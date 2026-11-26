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
	KindTaskExplanation  Kind = "task_explanation"
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

// TaskExplanation is why a task is on its reader's list.
func TaskExplanation(id ids.ActivityID) Subject {
	return Subject{kind: KindTaskExplanation, record: id.UUID}
}

// Entry is one stored summary and what its reader needs to judge it stale.
type Entry struct {
	Fingerprint string
	GeneratedAt time.Time
	GeneratedBy string
	Payload     []byte
}

// keySlot is the key column a kind is held in: company, contact, deal or
// task. The table's own CHECK demands that the others stay empty.
func keySlot(kind Kind) (int, error) {
	switch kind {
	case KindCompanyBrief, KindCompanyDossier, KindCompanyGrowthFit:
		return 0, nil
	case KindContactBrief:
		return 1, nil
	case KindDealStatusCard:
		return 2, nil
	case KindTaskExplanation:
		return 3, nil
	}
	return 0, fmt.Errorf("generated summary: %q is not a kind of summary", kind)
}

// ReadEachTx answers this reader's summaries of one kind, by the record each
// is about. A record with no stored summary is absent from the answer.
//
// The reader is part of the key because a summary is written from what that
// reader may see.
func ReadEachTx(
	ctx context.Context, tx pgx.Tx, reader ids.UserID, kind Kind, records []ids.UUID,
) (map[ids.UUID]Entry, error) {
	slot, err := keySlot(kind)
	if err != nil {
		return nil, err
	}
	var keys [4][]ids.UUID
	keys[slot] = records
	rows, err := tx.Query(ctx, `
		SELECT coalesce(company_id, contact_id, deal_id, activity_id),
		       fingerprint, generated_at, generated_by, payload
		  FROM generated_summary
		 WHERE user_id = $1 AND kind = $2
		   AND (company_id = ANY($3) OR contact_id = ANY($4) OR deal_id = ANY($5) OR activity_id = ANY($6))`,
		reader, kind, keys[0], keys[1], keys[2], keys[3])
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

// SaveTx replaces the summary stored for this reader.
func SaveTx(ctx context.Context, tx pgx.Tx, reader ids.UserID, subject Subject, written Entry) error {
	slot, err := keySlot(subject.kind)
	if err != nil {
		return err
	}
	var keys [4]*ids.UUID
	keys[slot] = &subject.record
	arguments := []any{
		reader, subject.kind, keys[0], keys[1], keys[2], keys[3],
		written.Fingerprint, written.GeneratedAt, written.GeneratedBy, written.Payload,
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO generated_summary (user_id, kind, company_id, contact_id, deal_id, activity_id,
		                               fingerprint, generated_at, generated_by, payload)
		VALUES (`+database.Placeholders(arguments)+`)
		ON CONFLICT (user_id, kind, company_id, contact_id, deal_id, activity_id) DO UPDATE
		SET fingerprint = EXCLUDED.fingerprint,
		    generated_at = EXCLUDED.generated_at,
		    generated_by = EXCLUDED.generated_by,
		    payload = EXCLUDED.payload`, arguments...); err != nil {
		return fmt.Errorf("store the generated summary: %w", err)
	}
	return nil
}

// Read is the reader's one summary of the subject, in its own transaction.
func Read(ctx context.Context, pool *pgxpool.Pool, reader ids.UserID, subject Subject) (Entry, bool, error) {
	var found map[ids.UUID]Entry
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		found, err = ReadEachTx(ctx, tx, reader, subject.kind, []ids.UUID{subject.record})
		return err
	})
	entry, stored := found[subject.record]
	return entry, stored, err
}

// Save is SaveTx in a transaction of its own.
func Save(ctx context.Context, pool *pgxpool.Pool, reader ids.UserID, subject Subject, written Entry) error {
	return database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return SaveTx(ctx, tx, reader, subject, written)
	})
}
