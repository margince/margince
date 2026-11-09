// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// The door that refuses a chat reading and the image policy that reads one
// agree on every type, both ways. A type only the door admits would be stored
// and never read.
func TestTheChatDoorAdmitsExactlyTheImagesTheReaderTakes(t *testing.T) {
	candidates := []string{
		"image/jpeg", "image/png", "image/gif", "image/webp", "image/tiff", "image/heic", "image/heif",
		"image/bmp", "image/svg+xml", "image/avif", "application/pdf", "text/plain",
	}
	for _, mime := range candidates {
		if slices.Contains(activities.ChatImageMIMEs(), mime) != ai.CarriesImage(mime) {
			t.Errorf("%s: the chat door and ai.CarriesImage disagree", mime)
		}
	}
	for _, mime := range activities.ChatImageMIMEs() {
		if !slices.Contains(candidates, mime) {
			t.Errorf("%s is admitted by the chat door and checked by nothing here", mime)
		}
	}
}
