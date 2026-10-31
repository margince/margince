// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What approving a staged action does, read off the approval card's preview.
// read_approval and the approval email both read a preview through ReadPreview,
// so neither shows less of the outcome than the card does.

import (
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
	State   string
	Note    string
	Changes []FieldChange
	Details []PreviewDetail
	Effects []PreviewEffect
}

const (
	previewStateReady = "ready"
	previewFieldBody  = "body"
)

// previewBodyFields are details the card shows as a message body, not a line.
var previewBodyFields = []string{previewFieldBody, "html_body", "signature_text", "signature_html"}

// PreviewWithholdsPlan reports a preview that shows nothing of the plan, as
// the card shows nothing of it. Whatever reads the plan elsewhere withholds it too.
func PreviewWithholdsPlan(preview *crmcontracts.ApprovalPreview) bool {
	return preview != nil && (!previewReadable(preview) || preview.State != previewStateReady)
}

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
	if PreviewWithholdsPlan(preview) {
		return PreviewReading{State: preview.State, Note: PreviewStateSentence(lang, preview.State)}
	}
	out := PreviewReading{State: preview.State}
	for _, change := range preview.Changes {
		out.Changes = append(out.Changes, FieldChange{
			Field: change.Field, Label: FieldLabel(lang, change.Field), Before: change.Before, After: change.After,
		})
	}
	if preview.Details != nil {
		for _, detail := range *preview.Details {
			if !slices.Contains(previewBodyFields, detail.Field) {
				out.Details = append(out.Details, PreviewDetail{Field: detail.Field, Label: FieldLabel(lang, detail.Field), Value: detail.Value})
			}
		}
	}
	if preview.Effects != nil {
		for _, effect := range *preview.Effects {
			out.Effects = append(out.Effects, PreviewEffect{Effect: effect, Label: EffectSentence(lang, effect)})
		}
	}
	return out
}
