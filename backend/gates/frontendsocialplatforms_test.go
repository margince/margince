// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The keys a contact's social map takes are decided on both sides of the wire.
// The server refuses a key outside socialplatform.Keys(). The edit form sends
// back only the keys its own list names. A key the server takes and the form
// drops is a link the form cannot carry. A key the form sends and the server
// refuses is a save that fails on every contact holding it.
//
// Both sides are read: the Go list from the package the write refuses against,
// the TypeScript array from the file the form reads. It fails in both
// directions.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/socialplatform"
)

const frontendSocialPlatforms = "../frontend/src/lib/socialplatforms.ts"

// tsPlatformEntry reads one quoted key out of the array literal, in either
// quote style, so an entry in the other style cannot parse as nothing.
var tsPlatformEntry = regexp.MustCompile(`["']([a-z0-9_]+)["']`)

func TestTheFrontendSocialPlatformsMatchTheGoList(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendSocialPlatforms)
	if err != nil {
		t.Fatalf("reading the frontend platform list: %v", err)
	}
	const marker = "SOCIAL_PLATFORMS = ["
	start := indexAfter(string(source), marker)
	if start < 0 {
		t.Fatalf("%s no longer declares %s as an array literal — this gate is reading a shape that is gone",
			frontendSocialPlatforms, marker)
	}
	end := indexAfter(string(source)[start:], "]")
	if end < 0 {
		t.Fatalf("%s's platform literal is unterminated", frontendSocialPlatforms)
	}
	literal := tsHostComment.ReplaceAllString(string(source)[start:start+end], " ")
	var inTS []string
	for _, m := range tsPlatformEntry.FindAllStringSubmatch(literal, -1) {
		inTS = append(inTS, m[1])
	}
	if len(inTS) == 0 {
		t.Fatal("no keys parsed out of the frontend array — a gate that reads nothing agrees with everything")
	}
	inGo := socialplatform.Keys()
	slices.Sort(inTS)
	slices.Sort(inGo)
	if strings.Join(inGo, ",") != strings.Join(inTS, ",") {
		t.Errorf("the social platform lists have drifted.\n  %s: %v\n  socialplatform.Keys(): %v\n"+
			"Change both in one commit.", frontendSocialPlatforms, inTS, inGo)
	}
}
