// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SourceProjection is shared by confirmation and its read-only preview.
type SourceProjection struct {
	PreviewToken         string     `json:"preview_token"`
	Rule                 NoticeRule `json:"rule"`
	NoticeRequired       bool       `json:"notice_required"`
	DeadlineIsUpperBound bool       `json:"deadline_is_upper_bound"`
	DueAt                *time.Time `json:"due_at"`
	Timezone             string     `json:"timezone"`
	duty                 NoticeDuty
}

func (in SourceAssessment) normalized() SourceAssessment {
	in.Note = strings.TrimSpace(in.Note)
	return in
}

func projectSourceAssessment(current NoticeCase, in SourceAssessment) (SourceProjection, error) {
	if current.Acquisition == nil {
		return SourceProjection{}, &ValidationError{Field: carrySourceArg, Reason: "the original acquisition evidence is unavailable"}
	}
	if current.State != NoticeSourceReview && (!current.needsSourceReview() || !current.sourceDecisionPending()) {
		return SourceProjection{}, &ValidationError{Field: fieldState, Reason: "only an unassessed source can be confirmed"}
	}
	duty, needed, err := in.validate()
	if err != nil {
		return SourceProjection{}, err
	}
	due, zone, err := in.deadline(duty, needed, current.Acquisition.CapturedAt)
	if err != nil {
		return SourceProjection{}, err
	}
	rule := duty.Rule
	if !needed {
		rule = RuleNone
	}
	return SourceProjection{Rule: rule, NoticeRequired: needed, DueAt: due, Timezone: zone, DeadlineIsUpperBound: needed && in.ObtainedAt == nil, duty: duty}, nil
}

func prepareSourceProjectionTx(ctx context.Context, tx pgx.Tx, current NoticeCase, in SourceAssessment) (SourceProjection, error) {
	projection, err := projectSourceAssessment(current, in)
	if err != nil {
		return SourceProjection{}, err
	}
	stamp := ""
	if in.SelectedEvidence != nil {
		evidence, err := resolveSourceEvidenceTx(ctx, tx, current, *in.SelectedEvidence)
		if err != nil {
			return SourceProjection{}, err
		}
		if evidence.Kind == sourceEvidenceAcquisition && current.Acquisition.Kind == acquiredUnknownLegacy {
			return SourceProjection{}, &ValidationError{Field: "selected_evidence", Reason: "the original record does not identify a source; select correspondence or describe the evidence"}
		}
		if evidence.Kind == sourceEvidenceAcquisition && current.Acquisition.Kind != in.Kind && in.Note == "" {
			return SourceProjection{}, &ValidationError{Field: fieldNote, Reason: "explain why the selected source supports a different answer"}
		}
		stamp, err = evidence.fingerprint()
		if err != nil {
			return SourceProjection{}, err
		}
	}
	raw, err := json.Marshal(struct {
		Case          ids.UUID
		Contact       ids.ContactID
		AcquisitionID ids.UUID
		State         NoticeState
		UpdatedAt     time.Time
		Owner         *ids.UUID
		Acquisition   *NoticeAcquisition
		Kind          string
		Note          string
		MetInPerson   bool
		ObtainedAt    *time.Time
		Evidence      *SourceEvidenceReference
		EvidenceStamp string
		Projection    SourceProjection
		Duty          NoticeDuty
	}{Case: current.ID, Contact: current.ContactID, AcquisitionID: current.AcquisitionID, State: current.State,
		UpdatedAt: current.UpdatedAt, Owner: current.OwnerUserID, Acquisition: current.Acquisition, Kind: in.Kind, Note: in.Note, MetInPerson: in.MetInPerson,
		ObtainedAt: in.ObtainedAt, Evidence: in.SelectedEvidence, EvidenceStamp: stamp, Projection: projection, Duty: projection.duty})
	if err != nil {
		return SourceProjection{}, fmt.Errorf("encode the source preview: %w", err)
	}
	_, projection.PreviewToken, err = diffhash.Canonical(raw)
	return projection, err
}

// PreviewNoticeSource runs the write's assessment without changing the case.
func (s *Store) PreviewNoticeSource(ctx context.Context, id ids.UUID, in SourceAssessment) (SourceProjection, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return SourceProjection{}, err
	}
	in = in.normalized()
	var out SourceProjection
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := readSourceCaseTx(ctx, tx, id)
		if err != nil {
			return err
		}
		out, err = prepareSourceProjectionTx(ctx, tx, current, in)
		return err
	})
	return out, err
}

func readSourceCaseTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (NoticeCase, error) {
	current, err := scanNoticeCase(tx.QueryRow(ctx, "SELECT"+noticeCaseColumns+" FROM privacy_notice_case WHERE id = @id", pgx.NamedArgs{"id": id}))
	if errors.Is(err, pgx.ErrNoRows) {
		return NoticeCase{}, apperrors.ErrNotFound
	}
	if err != nil {
		return NoticeCase{}, err
	}
	err = attachOneAcquisition(ctx, tx, &current)
	return current, err
}
