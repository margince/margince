// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func contactArchiveDetails(raw json.RawMessage) ([]crmcontracts.ApprovalPreviewDetail, error) {
	var contact crmcontracts.Contact
	if err := json.Unmarshal(raw, &contact); err != nil {
		return nil, err
	}
	var out []crmcontracts.ApprovalPreviewDetail
	if contact.Emails != nil {
		for _, email := range *contact.Emails {
			out = append(out, previewDetail("archive_email", string(email.Email)))
		}
	}
	if contact.Phones != nil {
		for _, phone := range *contact.Phones {
			out = append(out, previewDetail("archive_phone", phone.Phone))
		}
	}
	if contact.Reachability != nil {
		for _, channel := range *contact.Reachability {
			out = append(out, previewDetail("archive_channel", channel.Provider))
		}
	}
	return out, nil
}
