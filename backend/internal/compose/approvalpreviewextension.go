// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
)

// extensionPreview shows a staged extension call as the composed declaration
// names it plus every argument the approved call runs with. The approvals
// module shows the approval only while the unit row exists, and a unit row
// carries no version, so the display reads nothing the approval lacks.
func extensionPreview(approval crmcontracts.Approval, kind approvals.ExtensionKind, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	if approval.TargetEntityId == nil || approval.TargetEntityType == nil || *approval.TargetEntityType != kind.TargetTable ||
		approval.ProposedChange == nil || approval.Summary == nil {
		return out, nil
	}
	args := *approval.ProposedChange
	names := make([]string, 0, len(args))
	for name := range args {
		names = append(names, name)
	}
	sort.Strings(names)
	details := []crmcontracts.ApprovalPreviewDetail{previewDetail("summary", *approval.Summary)}
	for _, name := range names {
		text, isText := args[name].(string)
		if !isText {
			encoded, err := json.Marshal(args[name])
			if err != nil {
				return nil, err
			}
			text = string(encoded)
		}
		details = append(details, previewDetail(name, text))
	}
	out.Details, out.State = &details, previewReadyState
	return out, nil
}
