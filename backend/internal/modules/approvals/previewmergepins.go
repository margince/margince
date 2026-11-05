// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type mergePinRequest struct {
	Operation  string          `json:"operation"`
	Path       string          `json:"path"`
	Body       json.RawMessage `json:"body"`
	RecordType string          `json:"record_type"`
	SourceID   ids.UUID        `json:"source_id"`
	TargetID   ids.UUID        `json:"target_id"`
}

func previewMergePinsMatch(subject row, proposal crmcontracts.Approval) bool {
	if !mergePinsPresent(subject) {
		return false
	}
	raw, err := json.Marshal(proposal.ProposedChange)
	if err != nil {
		return false
	}
	var request mergePinRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		return false
	}
	if request.Operation != "" {
		operation, collection := "mergeContact", "contacts"
		if *subject.TargetType == tableCompany {
			operation, collection = "mergeCompany", "companies"
		}
		if request.Operation != operation || request.Path != "/v1/"+collection+"/"+subject.CoTargetID.String()+"/merge" {
			return false
		}
		var body mergePinRequest
		if err := json.Unmarshal(request.Body, &body); err != nil {
			return false
		}
		return body.TargetID == *subject.TargetID &&
			(body.SourceID.IsZero() || body.SourceID == *subject.CoTargetID)
	}
	return request.RecordType == *subject.TargetType &&
		request.SourceID == *subject.CoTargetID && request.TargetID == *subject.TargetID
}

func mergePinsPresent(subject row) bool {
	if subject.TargetType == nil || subject.TargetID == nil || subject.TargetVersion == nil ||
		subject.CoTargetType == nil || subject.CoTargetID == nil || subject.CoTargetVersion == nil ||
		*subject.CoTargetType != *subject.TargetType || subject.TargetID.IsZero() || subject.CoTargetID.IsZero() || *subject.TargetID == *subject.CoTargetID {
		return false
	}
	return true
}
