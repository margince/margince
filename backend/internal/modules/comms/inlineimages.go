// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/platform/emailview"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// InlineImages supplies the images a staged message's markup shows by content
// id. Compose binds it; this module owns neither the logo nor its storage.
type InlineImages interface {
	// SignatureLogo is the stored logo at key sized for mail, or false when
	// the object no longer exists because the workspace replaced or removed it.
	SignatureLogo(ctx context.Context, key string) (connector.InlineImage, bool, error)
}

// WithInlineImages binds the reader. Unbound, markup that refers to an image
// goes out without it, which is what an installation with no logo sends.
func (d *Dispatcher) WithInlineImages(images InlineImages) *Dispatcher {
	d.images = images
	return d
}

// inlineFor is the images this delivery's markup shows: the logo staged with
// it, never whichever logo the workspace has now. It is read before the
// delivery is marked in flight, so a failed read retries rather than sending a
// broken mail.
func (d *Dispatcher) inlineFor(ctx context.Context, del Delivery) ([]connector.InlineImage, error) {
	if d.images == nil || del.InlineLogoKey == "" {
		return nil, nil
	}
	logo, ok, err := d.images.SignatureLogo(ctx, del.InlineLogoKey)
	if err != nil || !ok {
		return nil, err
	}
	return []connector.InlineImage{logo}, nil
}

// markupFor is the markup that goes out beside these images. A staged logo the
// workspace has since deleted leaves its tag out, rather than a broken image.
func markupFor(del Delivery, inline []connector.InlineImage) string {
	if del.InlineLogoKey == "" || len(inline) > 0 {
		return del.HTMLBody
	}
	return strings.ReplaceAll(del.HTMLBody, connector.SignatureLogoTag, "")
}

// letterheadFor is the company logo a controller delivery's letterhead shows,
// under the content id that letterhead names.
func (d *Dispatcher) letterheadFor(ctx context.Context, del Delivery) ([]connector.InlineImage, error) {
	inline, err := d.inlineFor(ctx, del)
	if err != nil || len(inline) == 0 {
		return nil, err
	}
	inline[0].ContentID = connector.LetterheadLogoContentID
	return inline, nil
}

// letterheadMarkupFor is markupFor for a controller delivery: a staged logo the
// workspace has since removed leaves the company's name at the head.
func letterheadMarkupFor(del Delivery, markup string, inline []connector.InlineImage) string {
	if del.InlineLogoKey == "" || len(inline) > 0 {
		return markup
	}
	return emailview.WithoutLetterheadLogo(markup, connector.LetterheadLogoContentID)
}
