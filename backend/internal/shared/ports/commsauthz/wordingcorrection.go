// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// WordingCorrection binds a human's attestation to an observed message snapshot.
type WordingCorrection struct {
	Subject, Body, HTMLBody, ExpectedWording string
	Version                                  int64
	MeaningUnchanged                         bool
}

// WordingCorrectionReceipt links the owners' audit records without retaining message content.
type WordingCorrectionReceipt struct {
	InstructionID             ids.UUID
	BeforeDigest, AfterDigest string
}
