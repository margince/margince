// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// One reader of the installation's base language.
//
// The setting decides what language a message a stranger receives is written
// in, and two readers of it are two chances to disagree about which principal
// may ask and what an absent row means. Before this gate the tree had exactly
// that: identity's reset-mail path ran its own SELECT against the setting table
// while the confirm-mail path went through settings.ApplyTx, and the two
// spelled the fallback separately.
//
// The subject is the KEY, not a function name: any production statement that
// SELECTs on installation.base_language is a reader, wherever it lives and
// whatever it is called. LanguageOf is the one place allowed to, because it is
// where the setting is declared.
//
// Reads only. A harness or a fixture that INSERTs the key is seeding a value,
// not deciding what an absent one means, and sweeping those in would make this
// a waiver list rather than a finding.
//
// A second reader does not fail anything on its own — it returns the same value
// today. It fails the day one of them is changed and the other is not, which is
// a defect nobody will connect back to this, so it is refused up front.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// baseLanguageKey is the setting whose readers this gate counts.
const baseLanguageKey = "installation.base_language"

// readsTheLanguageKey reports whether a statement asks the setting table for
// this key, as opposed to seeding it.
func readsTheLanguageKey(statement string) bool {
	if !strings.Contains(statement, baseLanguageKey) {
		return false
	}
	upper := strings.ToUpper(statement)
	return strings.Contains(upper, "SELECT") &&
		!strings.Contains(upper, "INSERT") &&
		!strings.Contains(upper, "UPDATE")
}

// languageKeyOwners are the files that may name the key: the settings entry
// that declares it, and the contract types generated from the API description.
//
// gatekit:fixture why each file may name the base-language setting key
var languageKeyOwners = map[string]string{
	"internal/modules/identity/settingsentry.go": "declares the setting and is where LanguageOf reads it",
	"internal/contracts/api_gen.go":              "is generated from the contract, which publishes the key",
}

func TestTheInstallationLanguageHasOneReader(t *testing.T) {
	t.Parallel()

	var found []string
	eachGoFileInTheModule(t, func(path string, file *ast.File) {
		if _, owns := languageKeyOwners[path]; owns || strings.HasSuffix(path, "_test.go") {
			return
		}
		for _, text := range gatekit.SQLStatementsOf(file) {
			if readsTheLanguageKey(text) {
				found = append(found, path)
				break
			}
		}
	})

	for _, path := range found {
		t.Errorf("%s names %q itself rather than reading it through identity.LanguageOf. "+
			"Two readers of this setting are two spellings of what an absent row means and of "+
			"which principal may ask — and they answer the same thing today, so the disagreement "+
			"only appears once somebody changes one of them", path, baseLanguageKey)
	}
}
