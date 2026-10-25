// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The one rule that files a captured activity under the records of those on
// it, beyond the connector's refs and the counterparty the ladder judged.
//
// A participant row names a contact it resolved to, or an address. A lead is
// never resolved onto a participant row (ADR-0008), so a lead is matched by
// its exact address. A near miss names someone else, so nothing fuzzy matches.
//
// Mail, meetings and the lead backfill (leadfilingrepair.go) all read this rule.
// They differ only in their budget and in how a link is written.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// participantRecord is one record a participant row names.
type participantRecord struct {
	entity datasource.EntityType
	id     ids.UUID
}

// participantLinkColumns maps each record type this rule files under to its
// activity_link column. A type outside it is never formatted into SQL.
var participantLinkColumns = map[datasource.EntityType]string{
	datasource.EntityContact: "contact_id",
	datasource.EntityLead:    "lead_id",
}

// participantRecordsSQL answers the records the participant rows name, in the
// order the link ceiling cuts them. The organizer comes first, then contacts
// before leads, then ids, so a replay files the same way.
//
// A contact is followed through a merge before its liveness is judged. A merge
// archives the source, and no reader of activity_link walks merged_into_id.
//
// A lead counts only if the address was already its own when the activity
// happened. An address it took later belonged to someone else then.
var participantRecordsSQL = `
	SELECT entity_type, id FROM (
		SELECT 'contact' AS entity_type, 0 AS rank, survivor.id,
		       bool_or(ap.role = 'organizer') AS organized
		  FROM activity_participant ap
		  JOIN contact p ON p.id = ap.contact_id
		  JOIN contact survivor ON survivor.id = coalesce(p.merged_into_id, p.id)
		 WHERE ap.activity_id = $1 AND survivor.archived_at IS NULL
		 GROUP BY survivor.id
		UNION ALL
		SELECT 'lead', 1, ld.id, bool_or(ap.role = 'organizer')
		  FROM activity_participant ap
		  JOIN activity a ON a.id = ap.activity_id
		  JOIN lead ld ON ` + storekit.LeadEmailMatchClause("ld", "ap.address") + `
		 WHERE ap.activity_id = $1 AND ap.address IS NOT NULL
		   AND ` + storekit.LiveLeadClause("ld") + `
		   AND ` + storekit.LeadEmailHeldAtClause("ld", "ap.address", "a") + `
		 GROUP BY ld.id
	) named
	ORDER BY organized DESC, rank, id`

func participantRecords(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) ([]participantRecord, error) {
	rows, err := tx.Query(ctx, participantRecordsSQL, activityID)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the records an activity's participants name: %w", err)
	}
	defer rows.Close()
	var out []participantRecord
	for rows.Next() {
		var record participantRecord
		if err := rows.Scan(&record.entity, &record.id); err != nil {
			return nil, fmt.Errorf("capture: reading the records an activity's participants name: %w", err)
		}
		out = append(out, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading the records an activity's participants name: %w", err)
	}
	return out, nil
}

// mayFileUnder asks whether decider holds the record's read grant and may see
// its row (H1). A connector may not plant a link its granting human could not
// see. A refusal answers false rather than failing the capture of a message
// that was read successfully.
func mayFileUnder(decider context.Context, tx pgx.Tx, record participantRecord) (bool, error) {
	if !auth.ReadGranted(decider, string(record.entity)) {
		return false, nil
	}
	err := auth.EnsureLinkTarget(decider, tx, string(record.entity), record.id)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("capture: link target %s %s: %w", record.entity, record.id, err)
	}
	return true, nil
}

// participantFiling is how one caller spends the rule. It names who decides,
// the links left, the records asked about, and the write.
type participantFiling struct {
	decider context.Context
	budget  int
	// private says the message is private to the deciding seat, which files it
	// under no lead (messageIsPrivateTx).
	private bool
	// wants narrows the candidates; nil takes every one.
	wants func(participantRecord) bool
	// write files the activity under one admitted record and answers whether a
	// link landed.
	write func(participantRecord) (bool, error)
}

// fileUnderParticipants files the activity under each record its participants
// name, in order, until the budget is spent. It answers how many links landed.
func fileUnderParticipants(tx pgx.Tx, activityID ids.ActivityID, f participantFiling) (int, error) {
	records, err := participantRecords(f.decider, tx, activityID)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, record := range records {
		if written >= f.budget {
			break
		}
		if (f.wants != nil && !f.wants(record)) || (f.private && record.entity == datasource.EntityLead) {
			continue
		}
		admitted, err := mayFileUnder(f.decider, tx, record)
		if err != nil {
			return 0, err
		}
		if !admitted {
			continue
		}
		landed, err := f.write(record)
		if err != nil {
			return 0, err
		}
		if landed {
			written++
		}
	}
	return written, nil
}

// insertParticipantLink is the capture transaction's own write. The activity
// was minted in this transaction, and its create audit row covers the filing.
func insertParticipantLink(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) func(participantRecord) (bool, error) {
	return func(record participantRecord) (bool, error) {
		column, ok := participantLinkColumns[record.entity]
		if !ok {
			return false, fmt.Errorf("capture: activities cannot be filed under a participant's %s", record.entity)
		}
		tag, err := tx.Exec(ctx, storekit.SQLf(`
			INSERT INTO activity_link (activity_id, entity_type, %s)
			VALUES ($1, $2, $3)
			ON CONFLICT DO NOTHING`, column), activityID, string(record.entity), record.id)
		if err != nil {
			return false, fmt.Errorf("capture: filing an activity under its participant: %w", err)
		}
		return tag.RowsAffected() > 0, nil
	}
}
