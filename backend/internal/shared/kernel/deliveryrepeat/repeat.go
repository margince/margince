// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package deliveryrepeat is what an outbound delivery keeps so that a send can
// repeat it. The sending module fills it and the delivery store keeps it. This
// one type travels between them, so neither side copies it field by field.
package deliveryrepeat

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Repeat describes how a delivery can be sent again.
type Repeat struct {
	// BodyLen and HTMLLen count the characters at the front of the body and the
	// markup that the author wrote. The sign-off, disclosures and footer follow
	// them, and a repeat starts from the author's characters alone.
	BodyLen int
	HTMLLen int
	// Claims is what the send stated about why it was sent, as JSON.
	Claims []byte
	// From is the bounced delivery this one repeats, zero on any other.
	From ids.UUID
}
