// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// signatureLogoEdge is the widest edge of the logo a signature embeds: twice
// the 150px it is shown at, so it stays sharp on a dense screen.
const signatureLogoEdge = 300

// signatureLogoReadLimit bounds the stored logo read before resizing.
const signatureLogoReadLimit = 16 << 20

// signatureLogo hands the send path the workspace logo sized for mail. Each
// stored logo is resized once and kept by its object key, which changes with
// every upload, so a new logo is never served stale.
type signatureLogo struct {
	store *contacts.Store
	blob  blobstore.Store
	sized *sync.Map
}

func newSignatureLogo(store *contacts.Store, blob blobstore.Store) signatureLogo {
	return signatureLogo{store: store, blob: blob, sized: &sync.Map{}}
}

func (l signatureLogo) SignatureLogo(ctx context.Context) (connector.InlineImage, bool, error) {
	if l.blob == nil {
		return connector.InlineImage{}, false, nil
	}
	key, err := l.store.AnchorLogoKey(ctx)
	if err != nil || key == "" {
		return connector.InlineImage{}, false, err
	}
	if sized, ok := l.sized.Load(key); ok {
		if image, ok := sized.(connector.InlineImage); ok {
			return image, true, nil
		}
	}
	reader, _, err := l.blob.Get(ctx, key)
	if err != nil {
		return connector.InlineImage{}, false, fmt.Errorf("compose: reading the workspace logo: %w", err)
	}
	original, err := io.ReadAll(io.LimitReader(reader, signatureLogoReadLimit))
	if closeErr := reader.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return connector.InlineImage{}, false, fmt.Errorf("compose: reading the workspace logo: %w", err)
	}
	decoded, err := imagenorm.Decode(original)
	if err != nil {
		return connector.InlineImage{}, false, fmt.Errorf("compose: decoding the workspace logo: %w", err)
	}
	png, err := imagenorm.FitPNG(decoded, signatureLogoEdge)
	if err != nil {
		return connector.InlineImage{}, false, fmt.Errorf("compose: sizing the workspace logo for mail: %w", err)
	}
	image := connector.InlineImage{ContentID: connector.SignatureLogoContentID, ContentType: "image/png", Body: png}
	l.sized.Store(key, image)
	return image, true, nil
}
