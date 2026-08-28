// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The openchannel connector's failure vocabulary and its locale copy must name
// the SAME set of classes, or a member sees a raw translation key in place of a
// sentence for whichever class was renamed on one side and not the other.
//
// failureclasses.go is the SUBJECT: it is where the unit declares what can go
// wrong, and traffic.tsx renders `extOpenchannel.error.${row.error_class}`
// straight from that vocabulary with no fallback. A locale missing a declared
// class shows the reader a key instead of a sentence; a locale carrying a class
// nothing declares is dead copy nobody can trigger and a maintenance trap for
// the next rename.
//
// The declared set is read out of failureclasses.go's own `failureClasses`
// slice rather than hand-listed here, so this gate cannot drift out of step
// with the file it is supposed to hold: adding, renaming or removing a class
// there changes what every locale is required to carry without anyone having to
// remember to update a second list.

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

const (
	openchannelFailureClasses = "../extensions/openchannel/failureclasses.go"
	openchannelLocaleDir      = "../extensions/openchannel/frontend/i18n"
	openchannelErrorKeyPrefix = "extOpenchannel.error."
)

// declaredClass matches one `Class: "xxx",` field inside a FailureClass literal.
var declaredClass = regexp.MustCompile(`Class:\s*"([a-z_]+)"`)

// localeErrorKey matches one `"extOpenchannel.error.xxx": "..."` entry in a
// locale JSON file.
var localeErrorKey = regexp.MustCompile(`"extOpenchannel\.error\.([a-zA-Z0-9_]+)"\s*:`)

func TestOpenchannelLocalesNameExactlyTheDeclaredFailureClasses(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile(openchannelFailureClasses)
	if err != nil {
		t.Fatalf("reading %s: %v", openchannelFailureClasses, err)
	}
	matches := declaredClass.FindAllStringSubmatch(string(source), -1)
	if len(matches) == 0 {
		t.Fatalf("%s: no FailureClass.Class entries parsed — a gate that reads nothing agrees with everything", openchannelFailureClasses)
	}
	declared := map[string]bool{}
	for _, m := range matches {
		declared[m[1]] = true
	}

	entries, err := os.ReadDir(openchannelLocaleDir)
	if err != nil {
		t.Fatalf("reading %s: %v", openchannelLocaleDir, err)
	}
	found := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		found++
		path := filepath.Join(openchannelLocaleDir, entry.Name())
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("reading %s: %v", path, readErr)
		}
		present := map[string]bool{}
		for _, m := range localeErrorKey.FindAllStringSubmatch(string(body), -1) {
			present[m[1]] = true
		}

		for class := range declared {
			if !present[class] {
				t.Errorf("%s has no %s%s entry, but failureclasses.go declares it. "+
					"A member whose failure is this class sees the raw translation key instead of a sentence.",
					path, openchannelErrorKeyPrefix, class)
			}
		}
		for class := range present {
			if !declared[class] {
				t.Errorf("%s has %s%s, which failureclasses.go does not declare. "+
					"No failure can ever carry this class, so this entry is dead copy for a class that was renamed or removed.",
					path, openchannelErrorKeyPrefix, class)
			}
		}
	}
	if found == 0 {
		t.Fatalf("%s: no locale files found — a gate that reads no locale agrees with every one of them", openchannelLocaleDir)
	}
}
