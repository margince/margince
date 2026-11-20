// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The automation library reads in the reader's language. Each template, each
// parameter the rule form draws and each closed value has words. The Go side is
// the server's real catalog, so a parameter added only in Go fails here. Words
// for a key the catalog never has fail too.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

const (
	frontendCatalogWords = "../frontend/src/features/automations/automations.catalogwords.ts"
	frontendParamWords   = "../frontend/src/features/automations/automations.paramwords.ts"
	frontendInquiryLib   = "../frontend/src/lib/websiteinquiries.ts"
)

func TestEveryCatalogTemplateHasWords(t *testing.T) {
	t.Parallel()
	inGo := map[string]bool{}
	for _, entry := range automation.Catalog() {
		inGo[entry.Key] = true
	}
	entries := tsObjectEntries(t, frontendCatalogWords, "export const TEMPLATE_WORDS")
	for key, words := range entries {
		if !strings.Contains(words, "name:") || !strings.Contains(words, "description:") {
			t.Errorf("TEMPLATE_WORDS words %q without both a name and a description", key)
		}
	}
	wordsMatchVocabulary(t, "catalog key", "TEMPLATE_WORDS", inGo, tsKeySet(t, entries))
}

func TestEveryDrawnCatalogParameterHasWords(t *testing.T) {
	t.Parallel()
	assertTheInquiryRecipeIsTheOneTheFormSkips(t)
	params, options := drawnCatalogParams()
	wordsMatchVocabulary(t, "drawn catalog parameter", "PARAM_LABELS", params,
		tsKeySet(t, tsObjectEntries(t, frontendParamWords, "export const PARAM_LABELS")))

	optionLabels := tsObjectEntries(t, frontendParamWords, "export const OPTION_LABELS")
	for param := range tsKeySet(t, optionLabels) {
		if options[param] == nil {
			t.Errorf("OPTION_LABELS words values of %q, which no drawn parameter offers as a closed list", param)
		}
	}
	for _, param := range sortedKeys(options) {
		words, ok := optionLabels[param]
		if !ok {
			t.Errorf("parameter %q offers %v and OPTION_LABELS has no words for its values", param, sortedKeys(options[param]))
			continue
		}
		if strings.HasPrefix(words, "{") {
			body, _ := tsBalanced(words)
			wordsMatchVocabulary(t, "value of "+param, "OPTION_LABELS."+param, options[param], tsKeySet(t, tsLiteralEntries(body)))
			continue
		}
		// A map shared from elsewhere is owned there, so only its cover is asked.
		shared := tsImportedMapKeys(t, frontendParamWords, words)
		for _, value := range sortedKeys(options[param]) {
			if !shared[value] {
				t.Errorf("parameter %q offers %q and %s, its words, has none for it", param, value, words)
			}
		}
	}
}

const frontendRecipeWords = "../frontend/src/features/automations/automations.recipe.ts"

// Each trigger a catalog rule fires on has words in the library. A trigger the
// registry defines that no rule uses needs none.
func TestEveryCatalogTriggerHasWords(t *testing.T) {
	t.Parallel()
	inGo := map[string]bool{}
	for _, entry := range automation.Catalog() {
		inGo[entry.Trigger] = true
	}
	wordsMatchVocabulary(t, "catalog trigger", "TRIGGER", inGo,
		tsKeySet(t, tsObjectEntries(t, frontendRecipeWords, "const TRIGGER")))
}

// The inquiry recipe words its own fields in its own dialog, so the rule form
// and this gate both skip it, by the same key.
func assertTheInquiryRecipeIsTheOneTheFormSkips(t *testing.T) {
	t.Helper()
	source, err := gatekit.ReadFile(frontendInquiryLib)
	if err != nil {
		t.Fatalf("reading %s: %v", frontendInquiryLib, err)
	}
	match := regexp.MustCompile(`export const WEBSITE_INQUIRY_RECIPE = "([a-z_]+)"`).FindSubmatch(source)
	if match == nil || string(match[1]) != provenance.WebsiteInquirySource {
		t.Fatalf("%s's WEBSITE_INQUIRY_RECIPE is not %q, the recipe whose fields its own dialog words",
			frontendInquiryLib, provenance.WebsiteInquirySource)
	}
}

// drawnCatalogParams reads every top-level parameter the rule form draws, and
// the values each closed one offers. The form draws a scalar, a closed list,
// or a property a picker format names; a bare array or object it leaves out.
func drawnCatalogParams() (params map[string]bool, options map[string]map[string]bool) {
	params, options = map[string]bool{}, map[string]map[string]bool{}
	for _, entry := range automation.Catalog() {
		if entry.Key == provenance.WebsiteInquirySource {
			continue
		}
		properties, _ := entry.ParamsSchema["properties"].(map[string]any)
		for key, raw := range properties {
			property, _ := raw.(map[string]any)
			enum := schemaEnum(property["enum"])
			_, formatted := property["format"].(string)
			switch property["type"] {
			case "string", "integer", "number", "boolean":
			default:
				if !formatted && len(enum) == 0 {
					continue
				}
			}
			params[key] = true
			for _, value := range enum {
				if options[key] == nil {
					options[key] = map[string]bool{}
				}
				options[key][value] = true
			}
		}
	}
	return params, options
}

// schemaEnum reads a schema's closed list, built in Go as either slice type.
//
//craft:ignore naked-any a params schema is JSON-shaped, so its enum is whichever slice the builder wrote
func schemaEnum(raw any) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []any:
		var out []string
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

// tsImportedMapKeys reads the keys of a map that path imports by name.
func tsImportedMapKeys(t *testing.T, path, name string) map[string]bool {
	t.Helper()
	source, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	from := regexp.MustCompile(`import\s*\{[^}]*\b` + regexp.QuoteMeta(name) + `\b[^}]*\}\s*from\s*"([^"]+)"`).FindSubmatch(source)
	if from == nil {
		t.Fatalf("%s neither declares nor imports %s, which OPTION_LABELS names", path, name)
	}
	target := filepath.Join(filepath.Dir(path), string(from[1])) + ".ts"
	return tsKeySet(t, tsObjectEntries(t, target, "export const "+name))
}
