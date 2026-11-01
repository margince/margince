// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
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
	// touched collects the records a build read but names only as text.
	touched *previewTouched
}

func newApprovalPreviewReader(db *database.DB) approvalPreviewReader {
	return approvalPreviewReader{db: db, now: time.Now, names: newAttentionNames(db), records: NewProviderFor(db), deals: deals.NewStore(db, DealsInstallation()).WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(db)}), signals: signals.NewStore(db, nil), activities: activities.NewStore(db).WithChannelReachability(channelReachability{}).WithDisclosures(newDisclosureAdapter(db))}
}

// buildPreview dispatches to each kind's preview, reading as whichever principal ctx carries.
func (r approvalPreviewReader) buildPreview(ctx context.Context, approval crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
	if approval.Kind == "merge_members" {
		return r.memberMergePreview(ctx, approval)
	}
	if approval.Kind == CommitmentTaskKind {
		return r.commitmentPreview(ctx, approval)
	}
	out := &crmcontracts.ApprovalPreview{Version: 1, State: previewUnavailableState, Changes: []crmcontracts.ApprovalPreviewChange{}, ReviewerReason: previewReviewer(ctx, approval)}
	out.Evidence = &[]crmcontracts.ApprovalEvidence{}
	if directCompanyRead(approval) {
		return r.companyReadPreview(ctx, approval, out)
	}
	if build, own := previewBuilders[approval.Kind]; own {
		return build(r, ctx, approval, out)
	}
	if kind, registered := approvals.RegisteredExtensionKind(approval.Kind); registered {
		return extensionPreview(approval, kind, out)
	}
	return r.commercialPreview(ctx, approval, out)
}

// previewBuilders are the kinds whose preview reads its own proposal shape
// rather than a commercial field change.
var previewBuilders = map[string]func(approvalPreviewReader, context.Context, crmcontracts.Approval, *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error){
	heldScheduledSendKind: approvalPreviewReader.scheduledApprovalPreview,
	"coldstart":           approvalPreviewReader.coldStartPreview,
	toolMergeRecords:      approvalPreviewReader.mergePreview,
	operationMergeTags:    approvalPreviewReader.tagMergePreview,
	operationSetUpPartner: approvalPreviewReader.partnerSetUpPreview,
	operationCommitImport: approvalPreviewReader.importPreview,
}

func (r approvalPreviewReader) mergePreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	if approval.TargetEntityType != nil && *approval.TargetEntityType == captureCollisionTarget {
		return r.capturedLeadPreview(ctx, approval, out)
	}
	return r.recordMergePreview(ctx, approval, out)
}

func (r approvalPreviewReader) commercialPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	changes, covered, err := commercialChanges(approval)
	if !covered {
		if hasTypedProposalDisplay(approval.Kind) {
			return nil, nil //nolint:nilnil // These typed proposals retain their existing display and native decision guards.
		}
		return unsupportedApprovalPreview(out), nil
	}
	if previewUnavailable(err) {
		out.State = previewIncompleteState
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	if approval.Kind == toolCreateRecord {
		return r.creationPreview(ctx, approval, out)
	}
	if approval.Kind == previewSendCompanyEmail {
		return r.mailPreview(ctx, approval, out)
	}
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || approval.TargetVersion == nil {
		return out, nil
	}
	if approval.Kind == operationDemoteLead {
		return r.demotionPreview(ctx, approval, out)
	}
	return r.targetPreview(ctx, approval, changes, out)
}

func (r approvalPreviewReader) targetPreview(ctx context.Context, approval crmcontracts.Approval, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
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
	return r.recordPreview(ctx, approval, record, changes, out)
}

func (r approvalPreviewReader) recordPreview(ctx context.Context, approval crmcontracts.Approval, record datasource.Record, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	if approval.Kind == toolArchiveRecord {
		return r.archivePreview(ctx, approval, record.Fields, out)
	}
	if approval.Kind == singleRelinkTool || approval.Kind == operationRelinkActivities || approval.Kind == operationRelinkThread {
		return r.relinkPreview(ctx, approval, out)
	}
	if approval.Kind == operationPromoteLead {
		return r.promotionPreview(ctx, approval, out)
	}
	if approval.Kind == operationSendMessage {
		return r.channelPreview(ctx, approval, out)
	}
	if approval.Kind == operationInviteMeeting {
		return r.invitationPreview(ctx, approval, out)
	}
	if approval.Kind == operationBookMeeting {
		return r.bookingPreview(ctx, approval, out)
	}
	if approval.Kind == enrichProposalKind {
		return r.enrichmentPreview(ctx, approval, out)
	}
	if approval.Kind == previewSendEmail {
		return r.mailPreview(ctx, approval, out)
	}
	if approval.Kind == operationEmitFlowEvent {
		return r.confirmationPreview(ctx, approval, out)
	}
	return r.populateChanges(ctx, approval, record.Fields, changes, out)
}

func (r approvalPreviewReader) populateChanges(ctx context.Context, approval crmcontracts.Approval, fields json.RawMessage, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	current, err := previewCurrentFields(fields, changes)
	if err != nil {
		return nil, err
	}
	contextFields := maps.Clone(current)
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
	if err := r.dealDecisionContext(ctx, approval, contextFields, changes, out); err != nil {
		return nil, err
	}
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
	if detailed, ok := errors.AsType[*httperr.DetailedError](err); ok {
		switch detailed.Status {
		case http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusConflict, http.StatusUnprocessableEntity:
			return true
		}
	}
	var fieldFault apperrors.FieldFault
	var messageFault apperrors.MessageFault
	if errors.As(err, &fieldFault) || errors.As(err, &messageFault) {
		return true
	}
	return errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrInvalidArgument) || errors.Is(err, apperrors.ErrSeatTierInsufficient)
}

// The reasons previewReviewer gives, which the card and the approval email
// each turn into one sentence.
const (
	reviewerRequestedDecider = "requested_decider"
	reviewerOnBehalfOf       = "on_behalf_of"
	reviewerRecordAuthority  = "record_authority"
)

func previewReviewer(ctx context.Context, approval crmcontracts.Approval) string {
	actor, ok := principal.Actor(ctx)
	viewer := actor.UserID
	if !actor.OnBehalfOf.IsZero() {
		viewer = actor.OnBehalfOf
	}
	if ok && approval.RequestedDeciderId != nil && ids.UUID(*approval.RequestedDeciderId) == viewer {
		return reviewerRequestedDecider
	}
	if ok && approval.OnBehalfOf != nil && ids.UUID(*approval.OnBehalfOf) == viewer {
		return reviewerOnBehalfOf
	}
	return reviewerRecordAuthority
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
	value.Kind, value.Text = previewValueText, text
	switch field {
	case previewLifecycle:
		value.Kind = previewLifecycle
	case previewFieldValidUntil, previewFieldClosedAt, previewFieldStartedAt, previewFieldTargetEndDate, previewFXRateDate, acceptFieldExpectedClose:
		if _, err := time.Parse(time.RFC3339, text); err != nil {
			if _, err := time.Parse(time.DateOnly, text); err != nil {
				return value, apperrors.ErrInvalidArgument
			}
		}
		value.Kind, value.Text = previewValueDate, text
	case activityTargetOccurredAt:
		// A bare date is a day entry; a timestamp stays the instant it names.
		if _, err := time.Parse(time.DateOnly, text); err == nil {
			value.Kind = previewValueDate
		}
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
	if approval.Kind == operationDisqualifyLead {
		return r.disqualifyLeadChanges(ctx, approval, current, changes, out)
	}
	if approval.Kind == operationAdvanceProjectPhase {
		return r.projectPhaseChanges(ctx, approval, current, changes)
	}
	if !stagePreviewKind(approval.Kind) {
		return nil
	}
	if approval.Kind == deals.StageProgressionKind {
		if err := validateProgressionPreview(approval, current); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	var input struct {
		StageID    ids.StageID `json:"stage_id"`
		LostReason *string     `json:"lost_reason"`
		LostCode   *string     `json:"lost_reason_code"`
		WonReason  *string     `json:"won_without_contract_reason"`
		WonDetail  *string     `json:"won_without_contract_detail"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return apperrors.ErrInvalidArgument
	}
	preview, err := r.deals.PreviewAdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(*approval.TargetEntityId)), deals.AdvanceDealInput{
		ToStageID: input.StageID, LostReason: input.LostReason, LostReasonCode: input.LostCode, WonWithoutContractReason: input.WonReason,
		WonWithoutContractDetail: input.WonDetail, IfVersion: new(int64(*approval.TargetVersion)),
	})
	if err != nil {
		return err
	}
	out.Effects = &preview.Effects
	return applyPreviewChanges(current, changes, preview.Before, preview.After, bulkParamNote)
}

func applyPreviewChanges(current, changes map[string]json.RawMessage, beforeJSON, afterJSON json.RawMessage, preserve string) error {
	var before, after map[string]json.RawMessage
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		return err
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		return err
	}
	for field := range changes {
		if field != preserve {
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
	if !stagePreviewKind(kind) {
		return
	}
	for i := range changes {
		if changes[i].After.Kind == previewValueEmpty {
			continue
		}
		switch changes[i].Field {
		case previewFieldClosedAt, "fx_rate_to_base", previewFXRateDate, "amount_minor_base":
			changes[i].After = crmcontracts.ApprovalPreviewValue{Kind: "execution", Text: ""}
		}
	}
}
