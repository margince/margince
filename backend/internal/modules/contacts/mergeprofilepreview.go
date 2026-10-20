// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const mergeProfileColumns = `field, value_key, value, evidence_snippet, source_ref, confidence, source,
 captured_by, observed_at, superseded_value, superseded_captured_by, superseded_observed_at`

type MergeProfileAssertion struct {
	ID                   ids.UUID
	Field                string
	ValueKey             string
	Value                string
	EvidenceSnippet      string
	SourceRef            string
	Confidence           *float64
	Source               string
	CapturedBy           string
	ObservedAt           time.Time
	SupersededValue      *string
	SupersededCapturedBy *string
	SupersededObservedAt *time.Time
	UpdatedAt            time.Time
	Version              int64
}

type MergeProfileVerdict struct {
	Verdict        string
	Note           *string
	CorrectedValue *string
}

type MergeProfileValue struct {
	MergeProfileAssertion
	Verdict *MergeProfileVerdict
}

type ProfileVerdictReader func(context.Context, pgx.Tx, ids.UUID, []MergeProfileAssertion) ([]MergeProfileValue, error)

func (s *Store) WithProfileVerdictReader(read ProfileVerdictReader) *Store {
	s.profileVerdictReader = read
	return s
}

func (p *Provider) WithProfileVerdictReader(read ProfileVerdictReader) *Provider {
	p.store.WithProfileVerdictReader(read)
	return p
}

func (h Handlers) WithProfileVerdictReader(read ProfileVerdictReader) Handlers {
	h.store.WithProfileVerdictReader(read)
	return h
}

type MergeProfileField struct {
	Source MergeProfileValue
	Target *MergeProfileValue
}

func (s *Store) previewMergeProfileFields(ctx context.Context, tx pgx.Tx, source, target ids.UUID) ([]MergeProfileField, error) {
	from, err := s.readMergeProfileValues(ctx, tx, source, nil)
	if err != nil {
		return nil, err
	}
	if len(from) == 0 {
		return []MergeProfileField{}, nil
	}
	to, err := s.readMergeProfileValues(ctx, tx, target, &source)
	if err != nil {
		return nil, err
	}
	type fieldKey struct{ field, value string }
	kept := map[fieldKey]MergeProfileValue{}
	for _, value := range to {
		kept[fieldKey{value.Field, value.ValueKey}] = value
	}
	out := make([]MergeProfileField, 0, len(from))
	for _, value := range from {
		effect := MergeProfileField{Source: value}
		if survivor, found := kept[fieldKey{value.Field, value.ValueKey}]; found {
			effect.Target = &survivor
		}
		out = append(out, effect)
	}
	return out, nil
}

func (s *Store) readMergeProfileValues(ctx context.Context, tx pgx.Tx, contact ids.UUID, source *ids.UUID) ([]MergeProfileValue, error) {
	rows, err := tx.Query(ctx, `SELECT id, `+mergeProfileColumns+`, updated_at, version FROM contact_profile_field f
 WHERE contact_id = @contact AND (@source::uuid IS NULL OR EXISTS (SELECT 1 FROM contact_profile_field origin
 WHERE origin.contact_id = @source AND origin.field = f.field AND origin.value_key = f.value_key))
 ORDER BY field, value_key LIMIT 501`, pgx.StrictNamedArgs{"contact": contact, "source": source})
	if err != nil {
		return nil, err
	}
	values, err := pgx.CollectRows(rows, pgx.RowToStructByPos[MergeProfileAssertion])
	if err != nil {
		return nil, err
	}
	if len(values) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, value := range values {
		if raw, activity := strings.CutPrefix(value.SourceRef, "activity:"); activity {
			sourceID, err := ids.Parse(raw)
			if err != nil {
				return nil, apperrors.ErrInvalidArgument
			}
			if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
				return nil, err
			}
			if err := auth.EnsureActivityContentVisible(ctx, tx, sourceID); err != nil {
				return nil, err
			}
		}
		if err := previewFieldsVisible(ctx, "contact", value.Field); err != nil {
			return nil, err
		}
		switch value.Field {
		case "phone":
			if err := previewFieldsVisible(ctx, "contact", "phones"); err != nil {
				return nil, err
			}
		case "linkedin":
			if err := previewFieldsVisible(ctx, "contact", "social"); err != nil {
				return nil, err
			}
		case "first_name", "last_name":
			if err := previewFieldsVisible(ctx, "contact", "full_name"); err != nil {
				return nil, err
			}
		}
	}
	if len(values) == 0 {
		return []MergeProfileValue{}, nil
	}
	if s.profileVerdictReader == nil {
		return nil, apperrors.ErrInvalidArgument
	}
	return s.profileVerdictReader(ctx, tx, contact, values)
}
