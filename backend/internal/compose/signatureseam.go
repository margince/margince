// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// signatureReader hands the send path the sender's sign-off. contacts owns the
// row, the templates and the sender's legal entity; activities owns the send
// that renders them.
type signatureReader struct {
	store *contacts.Store
}

func (r signatureReader) SignatureFor(ctx context.Context, userID ids.UUID) (activities.SenderSignature, error) {
	signature, err := r.store.SignatureFor(ctx, userID)
	if err != nil {
		return activities.SenderSignature{}, err
	}
	return activities.SenderSignature{
		Body: signature.Body, HTML: signature.HTML, Title: signature.Title, Phone: signature.Phone, Template: signature.Template,
		Imprint: signature.Imprint, LogoKey: signature.LogoKey,
	}, nil
}
