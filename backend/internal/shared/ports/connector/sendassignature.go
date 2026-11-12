// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import "context"

// SendAsSignature is the signature a mailbox provider holds for the address
// the mailbox sends from by default, as the provider stores it.
type SendAsSignature struct {
	Address string
	HTML    string
}

// SendAsSignatureReader is how a connector reads that signature. Optional: a
// provider with no stored signature does not implement it.
type SendAsSignatureReader interface {
	SendAsSignature(ctx context.Context, auth Auth) (SendAsSignature, error)
}
