// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// PreviewMessage uses the frozen payload rather than rebuilding a send from current settings.
func (message ScheduledSend) PreviewMessage() (SendEmailInput, []ActivityLinkInput, error) {
	if message.PayloadVersion != payloadVersionCurrent {
		return SendEmailInput{}, nil, apperrors.ErrInvalidArgument
	}
	input, err := message.frozen.thaw()
	if err != nil {
		return SendEmailInput{}, nil, err
	}
	links, err := thawOriginLinks(message.additionalLinks)
	return input, links, err
}
