// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The browser names the server's body cap as a constant it cannot ask for,
// so this gate fails when either side moves without the other.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const frontendVoiceRefusal = "../frontend/src/lib/voice-refusal.ts"

var tsVoiceSourceCap = regexp.MustCompile(`(?m)^export const VOICE_SOURCE_MAX_BYTES = ([0-9_]+);`)

func TestTheVoiceSourceLimitTheBrowserNamesIsTheServerCap(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendVoiceRefusal)
	if err != nil {
		t.Fatalf("reading the frontend voice refusal vocabulary: %v", err)
	}
	match := tsVoiceSourceCap.FindStringSubmatch(string(source))
	if match == nil {
		t.Fatalf("%s no longer declares `export const VOICE_SOURCE_MAX_BYTES = <bytes>;`, so this gate reads nothing", frontendVoiceRefusal)
	}
	inBrowser, err := strconv.Atoi(strings.ReplaceAll(match[1], "_", ""))
	if err != nil {
		t.Fatalf("%s: %q is not a byte count", frontendVoiceRefusal, match[1])
	}
	if inBrowser != httperr.MaxBodyBytes {
		t.Fatalf("the voice step names a %d-byte limit, and the server reads a JSON write up to %d bytes (httperr.MaxBodyBytes): change both together",
			inBrowser, httperr.MaxBodyBytes)
	}
}
