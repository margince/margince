// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The brief's greeting and a drafted message must name a colleague by the same
// word. Both read a Vietnamese name's given name off one list of family names,
// so the TypeScript list is a declared mirror of the Go one. This compares both
// directions: a name on one side only fails.

import (
	"regexp"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

const frontendGreetingName = "../frontend/src/format/greetingname.ts"

var tsFamilyNameList = regexp.MustCompile(`(?s)VIETNAMESE_FAMILY_NAMES[^=]*=\s*new Set\(\[(.*?)\]\)`)

var tsFamilyName = regexp.MustCompile(`"([a-z]+)"`)

func TestTheFrontendFamilyNameListMatchesTheGoOne(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendGreetingName)
	if err != nil {
		t.Fatalf("reading the frontend greeting name rule: %v", err)
	}
	list := tsFamilyNameList.FindSubmatch(source)
	if list == nil {
		t.Fatalf("%s no longer declares VIETNAMESE_FAMILY_NAMES as a Set literal; this gate reads a shape that is gone", frontendGreetingName)
	}
	var inTS []string
	for _, m := range tsFamilyName.FindAllSubmatch(tsComment.ReplaceAll(list[1], nil), -1) {
		inTS = append(inTS, string(m[1]))
	}
	inGo := draftfloor.VietnameseFamilyNames()
	if len(inTS) == 0 {
		t.Fatal("no names parsed out of the frontend list; a gate that reads nothing agrees with everything")
	}
	slices.Sort(inTS)
	slices.Sort(inGo)
	if !slices.Equal(inTS, inGo) {
		t.Errorf("the family name lists differ:\nfrontend %v\nbackend  %v", inTS, inGo)
	}
}
