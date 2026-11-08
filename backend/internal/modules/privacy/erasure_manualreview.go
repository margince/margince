// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ErasureReviewSource names retained content without copying its personal text.
type ErasureReviewSource struct {
	Kind  string
	ID    ids.UUID
	Owner *ids.UUID
}

// SubjectReviewFlagger records manual follow-up beside the subject erasure.
type SubjectReviewFlagger interface {
	FlagSubjectReviewTx(context.Context, pgx.Tx, ids.ContactID, []ErasureReviewSource) error
}

const manualMentionSourcesSQL = `SELECT 'company',id,NULL::uuid FROM company WHERE description ~* @pattern
 UNION ALL SELECT 'deal',id,NULL::uuid FROM deal WHERE description ~* @pattern
 UNION ALL SELECT 'project',id,NULL::uuid FROM project WHERE description ~* @pattern
 UNION ALL SELECT 'legal_entity',id,NULL::uuid FROM legal_entity WHERE signature_template ~* @pattern
 UNION ALL SELECT 'email_signature',id,owner_id FROM email_signature WHERE body ~* @pattern OR html ~* @pattern OR title ~* @pattern OR phone ~* @pattern
 UNION ALL SELECT 'knowledge_document',d.id,NULL::uuid FROM knowledge_document d WHERE d.managed_source IS NULL
 AND (d.filename ~* @pattern OR EXISTS(SELECT 1 FROM knowledge_chunk c WHERE c.document_id=d.id AND c.text ~* @pattern))`

func (e *Eraser) flagManualSubjectMentions(ctx context.Context, tx pgx.Tx, subject ids.ContactID, keys subjectKeys) error {
	patterns := addressPatterns(keys.emails)
	name, named := values.FullNameTextPattern(keys.displayName, 1)
	_, full := values.FullNameTextPattern(keys.displayName, 2)
	if named && keys.displayName != erasedName {
		patterns = append(patterns, name)
	}
	var sources []ErasureReviewSource
	if len(patterns) > 0 {
		rows, err := tx.Query(ctx, manualMentionSourcesSQL, pgx.StrictNamedArgs{"pattern": strings.Join(patterns, "|")})
		if err != nil {
			return err
		}
		sources, err = pgx.CollectRows(rows, pgx.RowToStructByPos[ErasureReviewSource])
		if err != nil {
			return err
		}
	}

	if len(patterns) > 0 {
		rows, err := tx.Query(ctx, `SELECT 'activity',a.id,NULL::uuid FROM activity a WHERE a.kind='note'
 AND NOT EXISTS(SELECT 1 FROM activity_link l WHERE l.activity_id=a.id AND (l.contact_id IS NOT NULL OR l.lead_id IS NOT NULL))
 AND (@ambiguous OR a.restricted_at IS NOT NULL OR `+retentionscope.HeldThroughAnyLink("a.id")+`)
 AND (a.subject ~* @pattern OR a.body ~* @pattern)`, pgx.StrictNamedArgs{"pattern": strings.Join(patterns, "|"), "ambiguous": !full})
		if err != nil {
			return err
		}
		notes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[ErasureReviewSource])
		if err != nil {
			return err
		}
		sources = append(sources, notes...)
	}
	var template bool
	if len(patterns) > 0 {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM setting WHERE key='contacts.signature_template' AND value #>> '{}' ~* @pattern)`, pgx.StrictNamedArgs{"pattern": strings.Join(patterns, "|")}).Scan(&template); err != nil {
			return err
		}
	}
	if template {
		sources = append(sources, ErasureReviewSource{Kind: "signature_template", ID: subject.UUID})
	}

	if len(sources) == 0 {
		return nil
	}
	flagger, ok := e.purgeRawCaptures.(SubjectReviewFlagger)
	if !ok {
		return ErrRetentionSeamMissing
	}
	return flagger.FlagSubjectReviewTx(ctx, tx, subject, sources)
}
