// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importguide

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func TestEveryShippedLanguageWritesTheImportGuide(t *testing.T) {
	phrases := map[string]langcopy.Phrase{}
	for key, c := range Columns {
		phrases[key+".description"], phrases[key+".example"], phrases[key+".label"] = c.Description, c.Example, c.Label
	}
	for kind, c := range Custom {
		phrases["custom."+kind+".description"], phrases["custom."+kind+".example"] = c.Description, c.Example
	}
	for i, rule := range KeyRules {
		phrases[fmt.Sprintf("key_rule[%d]", i)] = rule
	}
	langcopytest.Census(t, phrases)
	langcopytest.Census(t, Template)
}

func TestEveryCustomFieldTypeHasAGuide(t *testing.T) {
	for _, kind := range fieldcatalog.Types() {
		if _, ok := Custom[kind]; !ok {
			t.Errorf("a %s custom field has no guide", kind)
		}
	}
}
