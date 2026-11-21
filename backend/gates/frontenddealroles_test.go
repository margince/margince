// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A stored deal role is free text, and both sides decide the same question.
// A built-in role is named, and a typed one is left out.
// The server answers it with dealrole.Label, the browser with dealRoleLabel.
//
// If the two lists differ, one role shows in the meeting brief and is missing
// on the contact page, or the reverse. This gate reads the browser's two label
// maps and compares them with the dealrole package in both directions.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/dealrole"
)

const frontendRecordLabels = "../frontend/src/lib/recordlabels.ts"

// tsLabelKey reads one `key: "message.key"` entry, bare or quoted.
var tsLabelKey = regexp.MustCompile(`(?m)^\s*["']?([a-z_]+)["']?:\s*"`)

func TestTheFrontendDealRolesMatchTheGoVocabulary(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendRecordLabels)
	if err != nil {
		t.Fatalf("reading the frontend role labels: %v", err)
	}
	maps := []struct {
		opens string
		want  []string
	}{
		{"const DEAL_ROLE_LABELS: Record<DealRole, MessageKey> = {", dealrole.Shown},
		{"const LEGACY_DEAL_ROLE_LABELS: Readonly<Record<string, MessageKey>> = {", []string{dealrole.LegacyDecisionMaker}},
	}
	for _, m := range maps {
		got := labelKeys(t, string(source), m.opens)
		want := append([]string{}, m.want...)
		sort.Strings(want)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s lists %v, dealrole lists %v: a role named on one side is left out on the other", m.opens, got, want)
		}
	}
}

// labelKeys reads the keys of the object literal that follows opens, with
// comments removed so a key named in a comment does not count.
func labelKeys(t *testing.T, source, opens string) []string {
	t.Helper()
	_, body, found := strings.Cut(source, opens)
	if !found {
		t.Fatalf("%s no longer declares %q; this gate reads a shape that is gone", frontendRecordLabels, opens)
	}
	declared, _, closed := strings.Cut(body, "};")
	if !closed {
		t.Fatalf("%s: the literal after %q is unterminated", frontendRecordLabels, opens)
	}
	literal := tsComment.ReplaceAllString(declared, " ")
	var keys []string
	for _, match := range tsLabelKey.FindAllStringSubmatch(literal, -1) {
		keys = append(keys, match[1])
	}
	if len(keys) == 0 {
		t.Fatalf("no keys parsed after %q; a gate that reads nothing agrees with everything", opens)
	}
	sort.Strings(keys)
	return keys
}
