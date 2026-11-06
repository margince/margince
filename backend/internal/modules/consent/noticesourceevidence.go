// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auditattribution"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// SourceEvidenceReference identifies a record without copying its content.
type SourceEvidenceReference struct {
	Kind string   `json:"kind"`
	ID   ids.UUID `json:"id"`
}

func (r SourceEvidenceReference) valid() bool {
	return r.Kind == sourceEvidenceAcquisition || r.Kind == entityActivity
}

// SourceEvidence carries only what this reader may see about a source.
type SourceEvidence struct {
	Reference         SourceEvidenceReference `json:"reference"`
	Kind              string                  `json:"kind"`
	Title             string                  `json:"title,omitempty"`
	OccurredAt        *time.Time              `json:"occurred_at"`
	OccurredPrecision string                  `json:"occurred_precision"`
	RecordedAt        time.Time               `json:"recorded_at"`
	CapturedBy        string                  `json:"captured_by,omitempty"`
	CapturedByName    *string                 `json:"captured_by_name,omitempty"`
	RecordType        string                  `json:"record_type"`
	RecordID          ids.UUID                `json:"record_id"`
	body              string
	updatedAt         time.Time
}

func (e SourceEvidence) fingerprint() (string, error) {
	e.CapturedByName = nil
	raw, err := json.Marshal(struct {
		Evidence  SourceEvidence
		Body      string
		UpdatedAt time.Time
	}{Evidence: e, Body: e.body, UpdatedAt: e.updatedAt})
	if err != nil {
		return "", fmt.Errorf("encode the selected source evidence: %w", err)
	}
	_, stamp, err := diffhash.Canonical(raw)
	return stamp, err
}

// SourceEvidencePage keeps the original source beside later correspondence.
type SourceEvidencePage struct {
	Original         *SourceEvidence          `json:"original,omitempty"`
	OriginalActivity *SourceEvidence          `json:"original_activity,omitempty"`
	Items            []SourceEvidence         `json:"items"`
	NextCursor       string                   `json:"next_cursor,omitempty"`
	SelectedEvidence *SourceEvidenceReference `json:"selected_evidence,omitempty"`
}

type sourceEvidenceCursor struct {
	At time.Time `json:"at"`
	ID ids.UUID  `json:"id"`
}

const sourceEvidencePageSize = 20
const sourceEvidenceAcquisition = "acquisition"

func sourceContactReadable(ctx context.Context, tx pgx.Tx, current NoticeCase) error {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return err
	}
	return auth.EnsureVisibleExisting(ctx, tx, entityContact, current.ContactID.UUID)
}

func resolveSourceEvidenceTx(ctx context.Context, tx pgx.Tx, current NoticeCase, ref SourceEvidenceReference) (SourceEvidence, error) {
	if err := sourceContactReadable(ctx, tx, current); err != nil {
		return SourceEvidence{}, err
	}
	if ref.Kind == sourceEvidenceAcquisition && ref.ID == current.AcquisitionID {
		var belongs bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM contact_acquisition_evidence WHERE id = @id AND contact_id = @contact)`, pgx.NamedArgs{"id": ref.ID, entityContact: current.ContactID}).Scan(&belongs); err != nil {
			return SourceEvidence{}, err
		}
		if !belongs || current.Acquisition == nil {
			return SourceEvidence{}, apperrors.ErrNotFound
		}
		a := current.Acquisition
		return SourceEvidence{Reference: ref, Kind: sourceEvidenceAcquisition, OccurredAt: a.OccurredAt, OccurredPrecision: "instant", RecordedAt: a.CapturedAt,
			CapturedBy: a.CapturedBy, CapturedByName: a.CapturedByName, RecordType: entityContact, RecordID: current.ContactID.UUID}, nil
	}
	if ref.Kind != entityActivity || ref.ID.IsZero() {
		return SourceEvidence{}, apperrors.ErrNotFound
	}
	rows, err := readSourceActivitiesTx(ctx, tx, current, &ref.ID, sourceEvidenceCursor{}, 1)
	if err != nil {
		return SourceEvidence{}, err
	}
	if len(rows) == 0 {
		return SourceEvidence{}, apperrors.ErrNotFound
	}
	return rows[0], nil
}

// GetNoticeSourceEvidence returns only records this reader can open.
func (s *Store) GetNoticeSourceEvidence(ctx context.Context, id ids.UUID, cursor string) (SourceEvidencePage, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return SourceEvidencePage{}, err
	}
	after, err := decodeSourceEvidenceCursor(cursor)
	if err != nil {
		return SourceEvidencePage{}, err
	}
	out := SourceEvidencePage{Items: []SourceEvidence{}}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := readSourceCaseTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := sourceContactReadable(ctx, tx, current); err != nil {
			return err
		}
		if current.Acquisition != nil {
			original, err := resolveSourceEvidenceTx(ctx, tx, current, SourceEvidenceReference{Kind: sourceEvidenceAcquisition, ID: current.AcquisitionID})
			if err != nil {
				return err
			}
			out.Original = &original
		}
		if err := attachSelectedSourceTx(ctx, tx, current, &out); err != nil {
			return err
		}
		if err := auth.Require(ctx, entityActivity, principal.ActionRead); errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		} else if err != nil {
			return err
		}
		if err := attachOriginalSourceActivityTx(ctx, tx, current, &out); err != nil {
			return err
		}
		out.Items, err = readSourceActivitiesTx(ctx, tx, current, nil, after, sourceEvidencePageSize+1)
		if err != nil {
			return err
		}
		if len(out.Items) > sourceEvidencePageSize {
			out.Items = out.Items[:sourceEvidencePageSize]
			last := out.Items[len(out.Items)-1]
			out.NextCursor, err = storekit.EncodeOpaque(sourceEvidenceCursor{At: *last.OccurredAt, ID: last.Reference.ID})
		}
		return err
	})
	redactSourceAttribution(ctx, &out)
	return out, err
}

// decodeSourceEvidenceCursor reads a page cursor; an empty one is the first page.
func decodeSourceEvidenceCursor(cursor string) (sourceEvidenceCursor, error) {
	if cursor == "" {
		return sourceEvidenceCursor{}, nil
	}
	decoded, err := storekit.DecodeOpaque[sourceEvidenceCursor](cursor)
	if err != nil || decoded.At.IsZero() || decoded.ID.IsZero() {
		return sourceEvidenceCursor{}, &storekit.MalformedCursorError{}
	}
	return decoded, nil
}

func redactSourceAttribution(ctx context.Context, out *SourceEvidencePage) {
	if !auditattribution.ReaderMayKnowWhoActed(ctx) {
		for _, evidence := range []*SourceEvidence{out.Original, out.OriginalActivity} {
			if evidence != nil {
				evidence.CapturedBy, evidence.CapturedByName = "", nil
			}
		}
		for i := range out.Items {
			out.Items[i].CapturedBy, out.Items[i].CapturedByName = "", nil
		}
	}
}

func attachSelectedSourceTx(ctx context.Context, tx pgx.Tx, current NoticeCase, page *SourceEvidencePage) error {
	if current.SourceEvidenceKind == nil || current.SourceEvidenceID == nil {
		return nil
	}
	ref := SourceEvidenceReference{Kind: *current.SourceEvidenceKind, ID: *current.SourceEvidenceID}
	if _, err := resolveSourceEvidenceTx(ctx, tx, current, ref); errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	} else if err != nil {
		return err
	}
	page.SelectedEvidence = &ref
	return nil
}

func attachOriginalSourceActivityTx(ctx context.Context, tx pgx.Tx, current NoticeCase, page *SourceEvidencePage) error {
	var source *ids.UUID
	err := tx.QueryRow(ctx, `SELECT source_entity_id FROM contact_acquisition_evidence
		WHERE id = @id AND contact_id = @contact AND source_entity_type = 'activity'`, pgx.NamedArgs{"id": current.AcquisitionID, entityContact: current.ContactID}).Scan(&source)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && source == nil) {
		return nil
	}
	if err != nil {
		return err
	}
	evidence, err := resolveSourceEvidenceTx(ctx, tx, current, SourceEvidenceReference{Kind: entityActivity, ID: *source})
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	page.OriginalActivity = &evidence
	return nil
}

func readSourceActivitiesTx(ctx context.Context, tx pgx.Tx, current NoticeCase, wanted *ids.UUID, cursor sourceEvidenceCursor, limit int) ([]SourceEvidence, error) {
	if err := auth.Require(ctx, entityActivity, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	contact, acquisition := arg(current.ContactID), arg(current.AcquisitionID)
	where := readable + " AND " + occurredday.HappenedSQL("a", "now()") + storekit.SQLf(` AND a.archived_at IS NULL
		AND (a.kind = 'meeting' OR (a.kind = 'email' AND NOT a.sender_is_machine AND NOT a.bulk_mail_attested))
		AND (EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.contact_id = $%[1]d)
		 OR EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = a.id AND p.contact_id = $%[1]d)
		 OR EXISTS (SELECT 1 FROM contact_acquisition_evidence e WHERE e.id = $%[2]d AND e.contact_id = $%[1]d AND e.source_entity_type = 'activity' AND e.source_entity_id = a.id))`, contact, acquisition)
	if wanted != nil {
		where += storekit.SQLf(" AND a.id = $%d", arg(*wanted))
	}
	if !cursor.ID.IsZero() {
		where += storekit.SQLf(" AND (a.occurred_at, a.id) < ($%d, $%d)", arg(cursor.At), arg(cursor.ID))
	}
	query := `SELECT a.id, a.kind, coalesce(a.subject, ''), a.occurred_at, a.occurred_precision, a.created_at, a.captured_by,
		u.display_name, coalesce(a.body, ''), a.updated_at
		FROM activity a LEFT JOIN app_user u ON a.captured_by = 'human:' || u.id::text AND ` + workforce.LiveMemberSQL("u") +
		` WHERE ` + where + storekit.SQLf(" ORDER BY a.occurred_at DESC, a.id DESC LIMIT $%d", arg(limit))
	if wanted != nil {
		query += " FOR SHARE OF a"
	}
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SourceEvidence{}
	for rows.Next() {
		e := SourceEvidence{RecordType: entityActivity, Reference: SourceEvidenceReference{Kind: entityActivity}}
		if err := rows.Scan(&e.Reference.ID, &e.Kind, &e.Title, &e.OccurredAt, &e.OccurredPrecision, &e.RecordedAt, &e.CapturedBy, &e.CapturedByName, &e.body, &e.updatedAt); err != nil {
			return nil, err
		}
		e.RecordID = e.Reference.ID
		out = append(out, e)
	}
	return out, rows.Err()
}
