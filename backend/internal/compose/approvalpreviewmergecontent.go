// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func mergeContentDetails(scope contacts.MergeSurvivorship) []crmcontracts.ApprovalPreviewDetail {
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, file := range scope.Files {
		details = append(details, mergeIdentityDetail("merge_file", file.Filename, file.ID), previewMoment("merge_file_archived", file.ArchivedAt))
	}
	if scope.Draft != nil {
		field := "merge_draft_moved"
		if scope.Draft.Target != nil {
			field = "merge_draft_discarded"
		}
		details = append(details, mergeDraftDetails(field, scope.Draft.Source)...)
		if scope.Draft.Target != nil {
			details = append(details, mergeDraftDetails("merge_draft_kept", *scope.Draft.Target)...)
		}
	}
	for _, item := range scope.Feedback {
		field := "merge_feedback_moved"
		if item.Target != nil {
			field = "merge_feedback_discarded"
		}
		details = append(details, mergeIdentityDetail(field, item.Source.ClaimKey, item.Source.ID), previewDetail("merge_feedback_verdict", item.Source.Verdict))
		if item.Source.ValueShown != nil {
			details = append(details, previewDetail("merge_feedback_value", *item.Source.ValueShown))
		}
		if item.Source.CorrectedValue != nil {
			details = append(details, previewDetail("merge_feedback_correction", *item.Source.CorrectedValue))
		}
		if item.Target != nil {
			details = append(details, mergeIdentityDetail("merge_feedback_kept", item.Target.ClaimKey, item.Target.ID), previewDetail("merge_feedback_verdict", item.Target.Verdict))
		}
	}
	for _, profile := range scope.ProfileFields {
		field := "merge_profile_moved"
		if profile.Target != nil {
			field = "merge_profile_discarded"
		}
		details = append(details, mergeIdentityDetail(field, profile.Source.Value, profile.Source.ID), previewDetail("merge_profile_field", profile.Source.Field))
		if profile.Target != nil {
			details = append(details, mergeIdentityDetail("merge_profile_kept", profile.Target.Value, profile.Target.ID))
		}
	}
	for _, run := range scope.ProviderRuns {
		details = append(details, previewDetail("merge_provider", run.Before.Provider), previewMoment("merge_run_created", &run.Before.CreatedAt), previewDetail("merge_run_before", string(run.Before.State)), previewDetail("merge_run_after", string(run.AfterState)), previewDetail("merge_run_skip", string(run.AfterSkipReason)), mergeBooleanDetail("merge_run_moved", run.Moves))
	}
	for _, signal := range scope.Signals {
		details = append(details, mergeIdentityDetail("merge_signal", signal.Summary, ids.UUID(signal.Id)))
	}
	return details
}

func mergeDraftDetails(field string, draft crmcontracts.MailDraft) []crmcontracts.ApprovalPreviewDetail {
	return []crmcontracts.ApprovalPreviewDetail{mergeIdentityDetail(field, draft.Subject, ids.UUID(draft.Id)), previewDetail("merge_draft_to", strings.Join(draft.To, ", ")), previewDetail("merge_draft_cc", strings.Join(draft.Cc, ", ")), previewDetail("merge_draft_bcc", strings.Join(draft.Bcc, ", ")), previewDetail("merge_draft_body", draft.Body)}
}
