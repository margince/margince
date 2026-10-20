// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"encoding/json"
	"strings"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type ConfirmationRequest struct {
	Question string `json:"question"`
	Reason   string `json:"reason,omitempty"`
}

func ReadConfirmationRequest(raw json.RawMessage) (ConfirmationRequest, error) {
	var input struct {
		ConfirmationRequest
		Note string `json:"note,omitempty"`
	}
	if err := datasource.StrictDecode(raw, &input); err != nil {
		return ConfirmationRequest{}, apperrors.ErrInvalidArgument
	}
	if input.Question != "" && input.Note != "" {
		return ConfirmationRequest{}, apperrors.ErrInvalidArgument
	}
	// The note field keeps stored confirmations readable.
	if input.Question == "" {
		input.Question = input.Note
	}
	if strings.TrimSpace(input.Question) == "" {
		return ConfirmationRequest{}, apperrors.ErrInvalidArgument
	}
	return input.ConfirmationRequest, nil
}
