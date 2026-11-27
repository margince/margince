// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "context"

func (h Handlers) bookingPrivacyInformation(ctx context.Context) (*string, error) {
	if h.publicConsent == nil {
		return nil, nil
	}
	address, err := h.publicConsent.BookingPrivacyInformation(ctx)
	if err != nil || address == "" {
		return nil, err
	}
	return &address, nil
}
