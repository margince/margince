// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The app's page loads images only from itself (img-src 'self' data:). A
// signature's https image would show as a broken icon in every preview. So the
// server fetches each one through the guarded web reader. The page gets a
// re-encoded copy, and the browser never contacts the image host. Only the
// preview read paths call this; a sent message carries the https address.

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"sync"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
	"github.com/margince/margince/backend/internal/platform/webread"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	// signaturePreviewMaxImages bounds the fetches one preview makes.
	signaturePreviewMaxImages = 5
	// signaturePreviewFetchBudget bounds how long a preview waits for its
	// images, decoding included. A slow host then costs a placeholder.
	signaturePreviewFetchBudget = 3 * time.Second
	// signaturePreviewMaxBytes and signaturePreviewMaxPixels refuse an image
	// before it is decoded, so one preview cannot hold a large canvas in memory.
	signaturePreviewMaxBytes  = 2 << 20
	signaturePreviewMaxPixels = 2_000_000
	// signaturePreviewCacheTTL is how long a copy, or a failed fetch, serves
	// repeat previews. A changed or revoked image shows within it.
	signaturePreviewCacheTTL = 10 * time.Minute
	// signaturePreviewCacheSize bounds the copies kept, because the composer
	// asks for a preview at every pause in typing.
	signaturePreviewCacheSize = 32
)

// signaturePreviewImages swaps https images for data: copies.
type signaturePreviewImages struct {
	fetch   assetFetcher
	now     func() time.Time
	mu      *sync.Mutex
	inlined map[previewImageKey]previewImageCopy
}

// previewImageKey scopes a copy to the workspace and the reader who fetched
// it. An address may answer each reader differently, so no reader is served
// bytes another reader's fetch got.
type previewImageKey struct {
	workspace, reader ids.UUID
	address           string
}

// previewImageCopy is a data: URL, or empty for an image that could not be
// drawn, which the placeholder then stands for until it expires.
type previewImageCopy struct {
	dataURL string
	at      time.Time
}

func newSignaturePreviewImages(fetch assetFetcher, now func() time.Time) *signaturePreviewImages {
	return &signaturePreviewImages{fetch: fetch, now: now, mu: &sync.Mutex{}, inlined: map[previewImageKey]previewImageCopy{}}
}

// sharedSignaturePreviewImages is the one cache every preview read shares.
var sharedSignaturePreviewImages = sync.OnceValue(func() *signaturePreviewImages {
	return newSignaturePreviewImages(webread.New(), time.Now)
})

// Inline returns the markup with each https image drawn from a data: copy. An
// image it cannot draw becomes a box of its size naming its alt text and host.
func (p *signaturePreviewImages) Inline(ctx context.Context, markup string) string {
	return activities.RewriteRemoteImages(markup, signaturePreviewMaxImages, func(addresses []string) map[string]string {
		return p.copies(ctx, addresses)
	})
}

// copies answers a data: URL for each address it could draw, fetching the
// ones not cached at the same time. An address it could not draw is absent.
func (p *signaturePreviewImages) copies(ctx context.Context, addresses []string) map[string]string {
	workspace, _ := principal.WorkspaceID(ctx)
	var reader ids.UUID
	if actor, ok := principal.Actor(ctx); ok {
		reader = actor.UserID
	}
	now := p.now()
	out := map[string]string{}
	var missing []string
	p.mu.Lock()
	for _, address := range addresses {
		if cached, ok := p.inlined[previewImageKey{workspace, reader, address}]; ok && now.Sub(cached.at) < signaturePreviewCacheTTL {
			out[address] = cached.dataURL
		} else if _, queued := out[address]; !queued {
			out[address] = ""
			missing = append(missing, address)
		}
	}
	p.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, signaturePreviewFetchBudget)
	defer cancel()
	fetched := make([]string, len(missing))
	var wait sync.WaitGroup
	for i, address := range missing {
		wait.Go(func() { fetched[i] = p.fetchCopy(ctx, address) })
	}
	wait.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, address := range missing {
		out[address] = fetched[i]
		if len(p.inlined) >= signaturePreviewCacheSize {
			clear(p.inlined)
		}
		p.inlined[previewImageKey{workspace, reader, address}] = previewImageCopy{dataURL: fetched[i], at: now}
	}
	return out
}

// fetchCopy gives up when the budget ends, so a slow fetch or decode costs
// the preview a placeholder and never its answer.
func (p *signaturePreviewImages) fetchCopy(ctx context.Context, address string) string {
	drawn := make(chan string, 1)
	go func() { drawn <- p.drawCopy(ctx, address) }()
	select {
	case dataURL := <-drawn:
		return dataURL
	case <-ctx.Done():
		return ""
	}
}

// drawCopy re-encodes the image as a PNG, so the page gets only bytes that
// decoded as an image and never the body the host served. The header is read
// first, so an oversized canvas is refused before it is decoded. Empty means
// the image could not be drawn; the placeholder says so.
func (p *signaturePreviewImages) drawCopy(ctx context.Context, address string) string {
	body, _, err := p.fetch.FetchAsset(ctx, address)
	if err != nil || len(body) > signaturePreviewMaxBytes {
		return ""
	}
	header, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || int64(header.Width)*int64(header.Height) > signaturePreviewMaxPixels {
		return ""
	}
	decoded, err := imagenorm.Decode(body)
	if err != nil {
		return ""
	}
	png, fits, err := mailSizedPNG(decoded)
	if err != nil || !fits {
		return ""
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
}
