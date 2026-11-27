// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

var (
	catalogCodeSpan     = regexp.MustCompile("`[^`]*`")
	catalogWireURL      = regexp.MustCompile(`(?:https?://|margince://)[^\s]+`)
	retiredCatalogWords = regexp.MustCompile(`(?i)\b(workspaces?|promises?|promised|verdicts?)\b`)
)

func catalogProseRetiredWords(text string) []string {
	text = catalogCodeSpan.ReplaceAllString(text, "")
	text = catalogWireURL.ReplaceAllString(text, "")
	return retiredCatalogWords.FindAllString(text, -1)
}

func TestThePublishedToolProseUsesTheProductVocabulary(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(publishedCatalog)
	if err != nil {
		t.Fatal(err)
	}
	var catalog any
	if err := json.Unmarshal(source, &catalog); err != nil {
		t.Fatal(err)
	}
	said := catalogStrings(catalog)
	if len(said) < catalogStringFloor {
		t.Fatalf("read only %d catalog strings", len(said))
	}
	for _, text := range sortedCatalogKeys(catalogProseStrings(catalog)) {
		if words := catalogProseRetiredWords(text); len(words) > 0 {
			t.Errorf("catalog prose says %v: %s; use company, commitment or result, and put a wire value in backticks", words, trimForFinding(text))
		}
	}
}

func TestCatalogVocabularyKeepsWireNamesAndRejectsRetiredProse(t *testing.T) {
	t.Parallel()
	for _, text := range []string{"Read the workspace.", "Open promises across the company.", "The classifier verdict is available.", "What was promised to customers?"} {
		if len(catalogProseRetiredWords(text)) == 0 {
			t.Errorf("accepted retired prose %q", text)
		}
	}
	for _, text := range []string{"open_promises", "Call `read_workspace` with scope `workspace`.", "Read margince://schema/workspace for company settings.", "Open commitments across the company."} {
		if words := catalogProseRetiredWords(text); len(words) > 0 {
			t.Errorf("refused wire or current prose %q: %v", text, words)
		}
	}
}

//craft:ignore naked-any the subject is the decoded published JSON tree.
func catalogProseStrings(node any) map[string]struct{} {
	found := map[string]struct{}{}
	var walk func(any)
	walk = func(node any) {
		switch value := node.(type) {
		case map[string]any:
			for key, child := range value {
				if text, ok := child.(string); ok && (key == "description" || key == "title" || key == "text") {
					found[text] = struct{}{}
				}
				walk(child)
			}
		case []any:
			for _, child := range value {
				walk(child)
			}
		}
	}
	walk(node)
	return found
}
