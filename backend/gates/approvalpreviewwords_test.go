// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// An approval's email and read_approval word a preview's effects and states as
// the approval card does.
//
// The card words them through the code maps in approvalpreview.ts, the email
// and the tool through agents/previewwords.go. A sentence that differs, or an
// effect only one side words, tells a reviewer two things about one approval.
// The TypeScript is read with a regexp, as approvalfieldlabels_test.go does.

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	cardPreviewSource     = "../frontend/src/lib/approvalpreview.ts"
	cardRetirementSource  = "../frontend/src/lib/memberretirement.ts"
	cardUnavailableKey    = "approval.preview.unavailable"
	cardUnavailableState  = "unavailable"
	cardRetirementEffects = "member_"
)

// cardCodeKey reads one `code: "catalog.key"` entry of a TypeScript code map.
var cardCodeKey = regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*"([A-Za-z0-9_.]+)",?\s*$`)

// catalogEntry reads one entry of a frontend catalog, whose value may sit on
// the next line when the formatter wraps it.
var catalogEntry = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*:\s*("(?:[^"\\\n]|\\.)*")`)

// cardCodeMap is the code map that starts at open and ends at its closing "};".
func cardCodeMap(t *testing.T, source, open string) map[string]string {
	t.Helper()
	start := strings.Index(source, open)
	if start < 0 {
		t.Fatalf("the card has no %q, so this gate reads nothing", open)
	}
	block := source[start:]
	if end := strings.Index(block, "};"); end >= 0 {
		block = block[:end]
	}
	out := map[string]string{}
	for _, m := range cardCodeKey.FindAllStringSubmatch(block, -1) {
		out[m[1]] = m[2]
	}
	if len(out) == 0 {
		t.Fatalf("%q holds no code this gate can read", open)
	}
	return out
}

// cardPreviewKeys is every effect and state the card words, with its catalog key.
func cardPreviewKeys(t *testing.T, preview string) (effects, states map[string]string) {
	t.Helper()
	effects = cardCodeMap(t, preview, "const PREVIEW_EFFECT_LABELS")
	for code, key := range cardCodeMap(t, readSource(t, cardRetirementSource), "const CONSEQUENCES") {
		effects[cardRetirementEffects+code] = key
	}
	states = cardCodeMap(t, preview, "const PREVIEW_STATE_LABELS")
	states[cardUnavailableState] = cardUnavailableKey
	return effects, states
}

func catalogSentences(t *testing.T, catalog string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, m := range catalogEntry.FindAllStringSubmatch(catalog, -1) {
		text, err := strconv.Unquote(m[2])
		if err != nil {
			t.Fatalf("%s holds %s, which this gate cannot read: %v", m[1], m[2], err)
		}
		out[m[1]] = text
	}
	return out
}

func TestAnApprovalWordsItsPreviewAsTheCardDoes(t *testing.T) {
	t.Parallel()
	effects, states := cardPreviewKeys(t, readSource(t, cardPreviewSource))
	for _, lang := range textlang.Shipped {
		catalog := catalogSentences(t, readSource(t, historyCatalog(lang)))
		for _, drift := range previewWordDrift(lang, catalog, effects, states) {
			t.Errorf("%s: %s", lang, drift)
		}
	}
}

// A sentence changed on the card, and an effect only the card words, must be seen.
func TestThePreviewWordGateSeesTheCardMoveAway(t *testing.T) {
	t.Parallel()
	preview := readSource(t, cardPreviewSource)
	effects, states := cardPreviewKeys(t, preview)
	catalog := catalogSentences(t, readSource(t, historyCatalog(textlang.English)))
	if len(previewWordDrift(textlang.English, catalog, effects, states)) != 0 {
		t.Fatal("the real catalogs already drift, so the plants prove nothing")
	}
	key := effects["promote_lead_archive_source"]
	if key == "" {
		t.Fatal("the card words no promote_lead_archive_source, so the plant proves nothing")
	}
	changed := maps.Clone(catalog)
	changed[key] += " Then something else."
	if len(previewWordDrift(textlang.English, changed, effects, states)) == 0 {
		t.Error("planted a changed effect sentence and the gate still agrees")
	}
	planted, _ := cardPreviewKeys(t, strings.Replace(preview,
		"const PREVIEW_EFFECT_LABELS: Record<string, MessageKey> = {",
		"const PREVIEW_EFFECT_LABELS: Record<string, MessageKey> = {\n  card_only_effect: \"approval.preview.stale\",", 1))
	if len(previewWordDrift(textlang.English, catalog, planted, states)) == 0 {
		t.Error("planted an effect only the card words and the gate still agrees")
	}
}

// previewWordDrift compares the card's sentence for every effect and state with
// the backend's, in both directions.
func previewWordDrift(lang textlang.Lang, catalog map[string]string, effects, states map[string]string) []string {
	var drift []string
	compare := func(kind string, card map[string]string, backend []string, word func(textlang.Lang, string) string) {
		for code, key := range card {
			if !slices.Contains(backend, code) {
				drift = append(drift, kind+" "+code+": the card words it and the email cannot")
				continue
			}
			if got, want := word(lang, code), catalog[key]; got != want {
				drift = append(drift, kind+" "+code+": the email says "+strconv.Quote(got)+", the card says "+strconv.Quote(want))
			}
		}
		for _, code := range backend {
			if _, ok := card[code]; !ok {
				drift = append(drift, kind+" "+code+": the email words it and the card does not")
			}
		}
	}
	compare("effect", effects, agents.PreviewEffects(), agents.EffectSentence)
	compare("state", states, agents.PreviewStates(), agents.PreviewStateSentence)
	return drift
}
