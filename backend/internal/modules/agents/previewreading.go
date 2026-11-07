// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What approving a staged action does, read off the approval card's preview.
// read_approval and the approval email both read a preview through ReadPreview,
// so neither shows less of the outcome than the card does.

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/approvalplan"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// PreviewDetail is one operand of a staged action, as the card lists it.
type PreviewDetail struct {
	Field string                            `json:"field"`
	Label string                            `json:"label"`
	Value crmcontracts.ApprovalPreviewValue `json:"value"`
}

// PreviewEffect is one further effect approving releases, in the card's words.
type PreviewEffect struct {
	Effect string `json:"effect"`
	Label  string `json:"label"`
}

// PreviewReading is a preview in lang. A preview that is not ready carries
// only its state and the card's sentence for it. The card then shows nothing
// of the plan either.
type PreviewReading struct {
	State      string
	Note       string
	Changes    []FieldChange
	Details    []PreviewDetail
	Effects    []PreviewEffect
	Conflicts  []crmcontracts.ApprovalPreviewConflict
	Files      []crmcontracts.ApprovalPreviewFile
	ImportRows []PreviewImportRow
}

// PreviewImportRow carries the same import outcome and operands as the card.
type PreviewImportRow struct {
	ObjectLabel  string                             `json:"object_label"`
	OutcomeLabel string                             `json:"outcome_label"`
	Line         int                                `json:"line"`
	Object       string                             `json:"object"`
	Outcome      string                             `json:"outcome"`
	SourceKey    string                             `json:"source_key"`
	Target       *crmcontracts.ApprovalPreviewValue `json:"target,omitempty"`
	Changes      []FieldChange                      `json:"changes"`
	Details      []PreviewDetail                    `json:"details"`
	Effects      []PreviewEffect                    `json:"effects"`
}

const previewFieldBody = "body"

// previewBodyFields are details the card shows as a message body, not a line.
var previewBodyFields = []string{previewFieldBody, "html_body", "signature_text", "signature_html"}

func previewReadable(preview *crmcontracts.ApprovalPreview) bool {
	return preview.Version == 1 || preview.Version == 2
}

// ReadPreview labels a preview the way the card does. A nil preview belongs to
// a kind with no preview, and reads as nothing.
func ReadPreview(lang textlang.Lang, preview *crmcontracts.ApprovalPreview) PreviewReading {
	if preview == nil {
		return PreviewReading{}
	}
	if !previewReadable(preview) {
		return PreviewReading{State: previewStateUnavailable, Note: PreviewStateSentence(lang, previewStateUnavailable)}
	}
	if approvalplan.Withheld(preview) {
		return PreviewReading{State: preview.State, Note: PreviewStateSentence(lang, preview.State)}
	}
	out := PreviewReading{State: preview.State}
	for _, change := range preview.Changes {
		out.Changes = append(out.Changes, FieldChange{
			Field: change.Field, Label: PreviewFieldLabel(lang, change.Field), Before: previewCodedValue(lang, change.Field, change.Before), After: previewCodedValue(lang, change.Field, change.After),
		})
	}
	if preview.Details != nil {
		for _, detail := range *preview.Details {
			if !slices.Contains(previewBodyFields, detail.Field) {
				out.Details = append(out.Details, PreviewDetail{Field: detail.Field, Label: PreviewFieldLabel(lang, detail.Field), Value: previewCodedValue(lang, detail.Field, detail.Value)})
			}
		}
	}
	if preview.Effects != nil {
		for _, effect := range *preview.Effects {
			out.Effects = append(out.Effects, PreviewEffect{Effect: effect, Label: EffectSentence(lang, effect)})
		}
	}
	if preview.Conflicts != nil {
		out.Conflicts = slices.Clone(*preview.Conflicts)
	}
	if preview.Files != nil {
		out.Files = slices.Clone(*preview.Files)
	}
	if preview.ImportRows != nil {
		for _, row := range *preview.ImportRows {
			nested := ReadPreview(lang, &crmcontracts.ApprovalPreview{Version: preview.Version, State: preview.State, Changes: row.Changes, Details: &row.Details, Effects: &row.Effects})
			out.ImportRows = append(out.ImportRows, PreviewImportRow{ObjectLabel: PreviewFieldLabel(lang, row.Object+"_id"), OutcomeLabel: previewImportOutcome(lang, string(row.Outcome)), Line: row.Line, Object: row.Object, Outcome: string(row.Outcome), SourceKey: row.SourceKey, Target: row.Target, Changes: nested.Changes, Details: nested.Details, Effects: nested.Effects})
		}
	}
	return out
}
