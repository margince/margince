// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Storing a company mark, and knowing without a decode that stored bytes need
// no crop. A list screen asks for one logo per row, so whatever the serve path
// spends per request it spends twenty-five times on one page load.

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/imagenorm"
)

// trimmedLogoSuffix ends the key of every mark PutLogo stored. Its presence is
// the promise that the bytes carry no transparent canvas, which is what lets
// streamLogo send them as they are. A key without it predates that promise.
const trimmedLogoSuffix = ".trimmed.png"

// tightLogoKeysCap bounds the memory the legacy-key set may hold. Emptying it
// when full costs one decode per key on the next read, never a wrong answer.
const tightLogoKeysCap = 10000

// PutLogo trims a normalized mark of transparent canvas and stores it, under
// base plus the suffix that says so, and answers the key it stored at. A Put
// failure still answers the key, because a partial object may sit there for
// the caller to collect.
func PutLogo(ctx context.Context, blob blobstore.Store, base string, png []byte) (string, error) {
	trimmed, err := imagenorm.TrimTransparentPNG(png)
	if err != nil {
		return "", fmt.Errorf("contacts: trimming a logo before it is stored: %w", err)
	}
	key := base + trimmedLogoSuffix
	if err := blob.Put(ctx, key, bytes.NewReader(trimmed), int64(len(trimmed)), imagenorm.ContentType); err != nil {
		return key, err
	}
	return key, nil
}

// storedTrimmed reports whether PutLogo stored the object at key.
func storedTrimmed(key string) bool { return strings.HasSuffix(key, trimmedLogoSuffix) }

// tightLogoKeys remembers legacy keys whose bytes this process has seen to be
// already trimmed, so a legacy mark costs one decode per process rather than
// one per request. It relies on the writers minting a fresh key per stored
// mark (compose/sitelogo.go), so after the first store the one write a legacy
// key sees is the trimmed write-back, which leaves it tight.
type tightLogoKeys struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

func newTightLogoKeys() *tightLogoKeys {
	return &tightLogoKeys{keys: make(map[string]struct{})}
}

func (t *tightLogoKeys) has(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	_, ok := t.keys[key]
	return ok
}

func (t *tightLogoKeys) add(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.keys) >= tightLogoKeysCap {
		clear(t.keys)
	}
	t.keys[key] = struct{}{}
}
