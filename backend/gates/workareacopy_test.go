// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The passport page names each work area, and shows what an agent is told in
// one that is off, in the words the server uses.
//
// TypeScript is read with a regexp, the trade frontendminorunits_test.go makes.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/workarea"
)

const workAreaCatalog = "../frontend/src/i18n/en.ts"

// enMessage is one key's English text, whether on the key's line or the next.
func enMessage(t *testing.T, catalog, key string) string {
	t.Helper()
	pattern := regexp.MustCompile(`"` + regexp.QuoteMeta(key) + `":\s*"([^"\n]*)"`)
	found := pattern.FindStringSubmatch(catalog)
	if found == nil {
		t.Fatalf("%s carries no %s", workAreaCatalog, key)
	}
	return found[1]
}

func TestThePassportPageSpeaksOfWorkAreasAsTheServerDoes(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(workAreaCatalog)
	if err != nil {
		t.Fatalf("reading %s: %v", workAreaCatalog, err)
	}
	catalog := string(raw)
	areas := workarea.Choosable()
	if len(areas) < 2 {
		t.Fatalf("the table offers %d areas", len(areas))
	}
	for _, area := range areas {
		if got := enMessage(t, catalog, "workArea.name."+area.Key); got != area.Name {
			t.Errorf("the page names %s %q and the server %q; make them one name", area.Key, got, area.Name)
		}
	}
	shown := strings.ReplaceAll(enMessage(t, catalog, "passportAreas.refusal"), "{area}", areas[0].Name)
	_, told := (&auth.AreaOffError{Area: areas[0].Key}).MessageFault()
	if want := strings.ToUpper(told[:1]) + told[1:] + "."; shown != want {
		t.Errorf("the panel shows an agent is told\n\t%s\nand the server tells it\n\t%s", shown, want)
	}
}
