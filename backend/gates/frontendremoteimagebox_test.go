// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The approval card offers the drawn signature only where the list's markup
// carries the box marker. A frontend constant that drifts from the server's
// attribute would find no box, and the reviewer would never be offered the
// images.

import (
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendRemoteImageBox = "../frontend/src/features/automations/remoteimagebox.ts"

var tsRemoteImageBox = regexp.MustCompile(`export const REMOTE_IMAGE_BOX\s*=\s*["']([^"']*)["']`)

func TestTheFrontendRemoteImageBoxMatchesTheGoOne(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendRemoteImageBox)
	if err != nil {
		t.Fatalf("reading the frontend image box marker: %v", err)
	}
	found := tsRemoteImageBox.FindAllStringSubmatch(string(source), -1)
	if len(found) != 1 {
		t.Fatalf("%s declares REMOTE_IMAGE_BOX %d times, want once: this gate reads a shape that is gone", frontendRemoteImageBox, len(found))
	}
	if got, want := found[0][1], activities.RemoteImageBoxAttribute; got != want {
		t.Fatalf("the frontend marks an undrawn image with %q and the server with %q: the card finds no box", got, want)
	}
}
