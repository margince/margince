// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"sort"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

const (
	previewProgressDeal     = "progress_deal"
	previewFXRateDate       = "fx_rate_date"
	previewUnavailableState = "unavailable"
	previewIncompleteState  = "incomplete"
	previewStaleState       = "stale"
	previewReadyState       = "ready"
	previewValueEmpty       = "empty"
	previewValueNumber      = "number"
	previewValueBoolean     = "boolean"
	previewValueText        = "text"
	previewValueDate        = "date"
	previewAdvanceDeal      = "advance_deal"
	previewLifecycle        = "lifecycle"
)

type approvalPreviewReader struct {
	now        func() time.Time
	db         *database.DB
	records    datasource.SystemOfRecordProvider
	deals      *deals.Store
	names      attentionNames
	labels     map[string]map[ids.UUID]string
	signals    *signals.Store
	activities *activities.Store
}

func newApprovalPreviewReader(db *database.DB) approvalPreviewReader {
	return approvalPreviewReader{db: db, now: time.Now, names: newAttentionNames(db), records: NewProviderFor(db), deals: deals.NewStore(db, DealsInstallation()), signals: signals.NewStore(db, nil), activities: activities.NewStore(db)}
}

func (r approvalPreviewReader) read(ctx context.Context, approval crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	if approval.Status != crmcontracts.ApprovalStatusPending {
		return nil, nil //nolint:nilnil // Terminal and uncovered kinds have no preview; their existing decision gates apply.
	}
	if approval.Kind == CommitmentTaskKind {
		return r.commitmentPreview(ctx, approval)
	}
	changes, covered, err := commercialChanges(approval)
	if !covered {
		return nil, nil //nolint:nilnil // Terminal and uncovered kinds have no preview; their existing decision gates apply.
	}
	out := &crmcontracts.ApprovalPreview{Version: 1, State: previewUnavailableState, Changes: []crmcontracts.ApprovalPreviewChange{}, ReviewerReason: previewReviewer(ctx, approval)}
	out.Evidence = &[]crmcontracts.ApprovalEvidence{}
	if previewUnavailable(err) {
		out.State = previewIncompleteState
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || approval.TargetVersion == nil {
		return out, nil
	}
	record, err := r.records.Read(ctx, datasource.EntityRef{Type: datasource.EntityType(*approval.TargetEntityType), ID: ids.UUID(*approval.TargetEntityId)})
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.CurrentVersion = new(int(record.Version))
	// A matching version proves the authorized current fields are the staged before values.
	// A stale row cannot reconstruct that snapshot and must not pretend it can.
	if record.Version != int64(*approval.TargetVersion) {
		out.State = previewStaleState
		return out, nil
	}
	return r.populateChanges(ctx, approval, record.Fields, changes, out)
}

func (r approvalPreviewReader) populateChanges(ctx context.Context, approval crmcontracts.Approval, fields json.RawMessage, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	current, err := previewCurrentFields(fields, changes)
	if err != nil {
		return nil, err
	}
	out.State = previewIncompleteState
	if err := r.stageChanges(ctx, approval, current, changes, out); err != nil {
		if errors.Is(err, apperrors.ErrVersionSkew) {
			out.State = previewStaleState
			return out, nil
		}
		if previewUnavailable(err) {
			return out, nil
		}
		return nil, err
	}
	if len(changes) == 0 {
		return out, nil
	}
	out.Changes, err = r.changedValues(ctx, approval.Kind, current, changes)
	if previewUnavailable(err) {
		out.Changes = []crmcontracts.ApprovalPreviewChange{}
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	markExecutionValues(approval.Kind, out.Changes)
	out.State = previewReadyState
	if err := r.currentEvidence(ctx, approval, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r approvalPreviewReader) changedValues(ctx context.Context, kind string, current, changes map[string]json.RawMessage) ([]crmcontracts.ApprovalPreviewChange, error) {
	labels, err := r.referenceNames(ctx, current, changes)
	if err != nil {
		return nil, err
	}
	r.labels = labels
	proposed := maps.Clone(current)
	maps.Copy(proposed, changes)
	keys := make([]string, 0, len(changes))
	for key := range changes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]crmcontracts.ApprovalPreviewChange, 0, len(keys))
	for _, key := range keys {
		before, visible := current[key]
		if key == bulkParamNote && kind == previewProgressDeal {
			before, visible = json.RawMessage(jsonNull), true
		}
		if !visible {
			return nil, apperrors.ErrPermissionDenied
		}
		old, err := r.value(ctx, key, before)
		if key == bulkParamOwner && errors.Is(err, apperrors.ErrNotFound) {
			old, err = crmcontracts.ApprovalPreviewValue{Kind: "unavailable_name"}, nil
		}
		if err != nil {
			return nil, err
		}
		next, err := r.value(ctx, key, changes[key])
		if err != nil {
			return nil, err
		}
		if err := r.moneyValue(ctx, key, &old, current); err != nil {
			return nil, err
		}
		if err := r.moneyValue(ctx, key, &next, proposed); err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.ApprovalPreviewChange{Field: key, Before: old, After: next})
	}
	return out, nil
}

func previewUnavailable(err error) bool {
	var fieldFault apperrors.FieldFault
	var messageFault apperrors.MessageFault
	if errors.As(err, &fieldFault) || errors.As(err, &messageFault) {
		return true
	}
	return errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrInvalidArgument)
}

func previewReviewer(ctx context.Context, approval crmcontracts.Approval) string {
	actor, ok := principal.Actor(ctx)
	viewer := actor.UserID
	if !actor.OnBehalfOf.IsZero() {
		viewer = actor.OnBehalfOf
	}
	if ok && approval.RequestedDeciderId != nil && ids.UUID(*approval.RequestedDeciderId) == viewer {
		return "requested_decider"
	}
	if ok && approval.OnBehalfOf != nil && ids.UUID(*approval.OnBehalfOf) == viewer {
		return "on_behalf_of"
	}
	return "record_authority"
}

func (r approvalPreviewReader) value(ctx context.Context, field string, raw json.RawMessage) (crmcontracts.ApprovalPreviewValue, error) {
	raw = bytes.TrimSpace(raw)
	value := crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty, Text: ""}
	if string(raw) == jsonNull {
		return value, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return previewScalar(raw)
	}
	value.Kind, value.Text = previewValueText, approvals.SanitizeSummary(text)
	switch field {
	case previewLifecycle:
		value.Kind = previewLifecycle
	case "closed_at", previewFXRateDate, acceptFieldExpectedClose:
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			if _, err := time.Parse(time.DateOnly, text); err != nil {
				return value, apperrors.ErrInvalidArgument
			}
		}
		value.Kind, value.Text = previewValueDate, text
	default:
		if strings.HasSuffix(field, "_id") {
			return r.referenceValue(ctx, field, text)
		}
	}
	return value, nil
}

func previewScalar(raw json.RawMessage) (crmcontracts.ApprovalPreviewValue, error) {
	value := crmcontracts.ApprovalPreviewValue{Kind: previewValueEmpty}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err == nil && number != "" {
		return crmcontracts.ApprovalPreviewValue{Kind: previewValueNumber, Text: number.String()}, nil
	}
	var flag bool
	if err := json.Unmarshal(raw, &flag); err == nil {
		return crmcontracts.ApprovalPreviewValue{Kind: previewValueBoolean, Text: strconv.FormatBool(flag)}, nil
	}
	if len(raw) > 0 && (raw[0] == '{' || raw[0] == '[') {
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err != nil {
			return value, apperrors.ErrInvalidArgument
		}
		return crmcontracts.ApprovalPreviewValue{Kind: "structured", Text: compact.String()}, nil
	}
	return value, apperrors.ErrInvalidArgument
}

func previewCurrentFields(raw json.RawMessage, changes map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, apperrors.ErrInvalidArgument
	}
	for field := range changes {
		if _, present := fields[field]; !present {
			fields[field] = json.RawMessage(jsonNull)
		}
	}
	if masked, ok := fields["masked_fields"]; ok {
		var names []string
		if err := json.Unmarshal(masked, &names); err != nil {
			return nil, err
		}
		// A withheld null is not an empty before value.
		for _, name := range names {
			delete(fields, name)
		}
	}
	return fields, nil
}

func (r approvalPreviewReader) stageChanges(ctx context.Context, approval crmcontracts.Approval, current, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) error {
	if approval.Kind != previewAdvanceDeal && approval.Kind != previewProgressDeal {
		return nil
	}
	raw, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	var input struct {
		StageID    ids.StageID `json:"stage_id"`
		LostReason *string     `json:"lost_reason"`
		WonReason  *string     `json:"won_without_contract_reason"`
		WonDetail  *string     `json:"won_without_contract_detail"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return apperrors.ErrInvalidArgument
	}
	preview, err := r.deals.PreviewAdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(*approval.TargetEntityId)), deals.AdvanceDealInput{
		ToStageID: input.StageID, LostReason: input.LostReason, WonWithoutContractReason: input.WonReason,
		WonWithoutContractDetail: input.WonDetail, IfVersion: new(int64(*approval.TargetVersion)),
	})
	if err != nil {
		return err
	}
	out.Effects = &preview.Effects
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(preview.Before, &before); err != nil {
		return err
	}
	if err := json.Unmarshal(preview.After, &after); err != nil {
		return err
	}
	for field := range changes {
		if field != bulkParamNote {
			delete(changes, field)
		}
	}
	for field, value := range after {
		if bytes.Equal(before[field], value) {
			continue
		}
		changes[field] = value
		current[field] = before[field]
	}
	return nil
}

func markExecutionValues(kind string, changes []crmcontracts.ApprovalPreviewChange) {
	if kind != previewAdvanceDeal && kind != previewProgressDeal {
		return
	}
	for i := range changes {
		if changes[i].After.Kind == previewValueEmpty {
			continue
		}
		switch changes[i].Field {
		case "closed_at", "fx_rate_to_base", previewFXRateDate, "amount_minor_base":
			changes[i].After = crmcontracts.ApprovalPreviewValue{Kind: "execution", Text: ""}
		}
	}
}
