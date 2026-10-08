// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// InlineImages supplies the images a staged message's markup shows by content
// id. Compose binds it; this module owns neither the logo nor its storage.
type InlineImages interface {
	// SignatureLogo is the workspace logo sized for mail, or false when the
	// workspace has none.
	SignatureLogo(ctx context.Context) (connector.InlineImage, bool, error)
}

// WithInlineImages binds the reader. Unbound, markup that refers to an image
// goes out without it, which is what an installation with no logo sends.
func (d *Dispatcher) WithInlineImages(images InlineImages) *Dispatcher {
	d.images = images
	return d
}

// inlineFor is the images this markup shows. It is read before the delivery is
// marked in flight, so a failed read retries rather than sending a broken mail.
func (d *Dispatcher) inlineFor(ctx context.Context, html string) ([]connector.InlineImage, error) {
	if d.images == nil || !strings.Contains(html, "cid:"+connector.SignatureLogoContentID) {
		return nil, nil
	}
	logo, ok, err := d.images.SignatureLogo(ctx)
	if err != nil || !ok {
		return nil, err
	}
	return []connector.InlineImage{logo}, nil
}
