// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// signatureLogoEdge is the widest edge of the logo a signature embeds. It is
// twice the 150px the logo is shown at, so it stays sharp on a dense screen.
const signatureLogoEdge = 300

// signatureLogoMaxBytes is the most a signature logo adds to every message. A
// photo-like logo can pass it at 300px, so it is refitted smaller until it fits.
const signatureLogoMaxBytes = 48 << 10

// signatureLogoEdges are the edges tried in turn; the last one is sent even
// when it is still over the cap, since a smaller logo is no longer legible.
var signatureLogoEdges = []int{signatureLogoEdge, 200, 150}

// signatureLogoReadLimit bounds the stored logo read before resizing.
const signatureLogoReadLimit = 16 << 20

// signatureLogoCacheSize bounds the sized logos kept in memory. Every upload
// mints a new key, so without a bound each replaced logo would stay resident.
const signatureLogoCacheSize = 16

// signatureLogo hands the send path a stored logo sized for mail. A key names
// one immutable upload, so a sized copy kept by key is never stale.
type signatureLogo struct {
	blob  blobstore.Store
	mu    *sync.Mutex
	sized map[string]connector.InlineImage
}

func newSignatureLogo(blob blobstore.Store) signatureLogo {
	return signatureLogo{blob: blob, mu: &sync.Mutex{}, sized: map[string]connector.InlineImage{}}
}

func (l signatureLogo) SignatureLogo(ctx context.Context, key string) (connector.InlineImage, bool, error) {
	if l.blob == nil {
		return connector.InlineImage{}, false, nil
	}
	l.mu.Lock()
	cached, ok := l.sized[key]
	l.mu.Unlock()
	if ok {
		return cached, true, nil
	}
	reader, _, err := l.blob.Get(ctx, key)
	if errors.Is(err, blobstore.ErrNotFound) {
		return connector.InlineImage{}, false, nil
	}
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
	var png []byte
	for _, edge := range signatureLogoEdges {
		if png, err = imagenorm.FitPNG(decoded, edge); err != nil {
			return connector.InlineImage{}, false, fmt.Errorf("compose: sizing the workspace logo for mail: %w", err)
		}
		if len(png) <= signatureLogoMaxBytes {
			break
		}
	}
	image := connector.InlineImage{ContentID: connector.SignatureLogoContentID, ContentType: "image/png", Body: png}
	l.mu.Lock()
	if len(l.sized) >= signatureLogoCacheSize {
		clear(l.sized)
	}
	l.sized[key] = image
	l.mu.Unlock()
	return image, true, nil
}
