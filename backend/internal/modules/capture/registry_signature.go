// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ErrConnectorHasNoSignature is a connector that keeps no send-as signature.
var ErrConnectorHasNoSignature = errors.New("capture: this connector keeps no send-as signature")

// SignatureReaderFor resolves one user's live connection to provider for a
// read of its send-as signature, with the scopes the provider says it holds.
//
//nolint:ireturn // returns the optional connector.SendAsSignatureReader seam by design, as SenderFor does
func (r *Registry) SignatureReaderFor(ctx context.Context, userID ids.UserID, provider string) (connector.SendAsSignatureReader, connector.Auth, []string, error) {
	var reader connector.SendAsSignatureReader
	auth, granted, err := r.liveConnection(ctx, userID, provider, func(c connector.Connector) error {
		var reads bool
		if reader, reads = c.(connector.SendAsSignatureReader); !reads {
			return fmt.Errorf("capture: connector %q: %w", provider, ErrConnectorHasNoSignature)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return reader, auth, granted, nil
}
