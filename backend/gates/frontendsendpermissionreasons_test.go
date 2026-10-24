// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The composer has its own sentence for every code the send engine refuses with, and no other.
//
// sendPermissionReasonKey sends a code it has no case for to the generic refusal, which names
// neither the cause nor the fix. A code the server dropped leaves a case nothing can reach.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

var frontendSendPermissionReasons = filepath.Join(repoRoot, "frontend", "src", "format", "sendpermissionreason.ts")

const (
	sendPermissionReasonMapper = "sendPermissionReasonKey"
	serverRefusalCodes         = "commsauthz.RefusalCodes in backend/internal/shared/ports/commsauthz/refusalcodes.go"
)

var (
	frontendI18nDir  = filepath.Join(repoRoot, "frontend", "src", "i18n")
	frontendCatalogs = []string{"en.ts", "de.ts", "vi.ts"}
)

// tsCaseKeyword counts the case statements, and tsCaseLabel the ones this gate can read.
// A case or return it cannot read fails the gate rather than shrinking the census.
var (
	tsMapperDeclaration = regexp.MustCompile(`\bfunction\s+` + sendPermissionReasonMapper + `\s*\(`)
	tsCaseKeyword       = regexp.MustCompile(`\bcase\b`)
	tsCaseLabel         = regexp.MustCompile("\\b(?:case\\s+(?:\"([^\"]*)\"|'([^']*)'|`([^`]*)`)|(default))\\s*:")
	tsReturnKeyword     = regexp.MustCompile(`\breturn\b`)
	tsReturnStatement   = regexp.MustCompile(`\breturn\b([^;]*);`)
	tsReturnedLiteral   = regexp.MustCompile("\"([^\"]*)\"|'([^']*)'|`([^`]*)`")
	// A key starts its own line; a commented-out line starts with // and is not read.
	tsCatalogKey   = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*:`)
	tsBlockComment = regexp.MustCompile(`(?ms)^\s*/\*.*?\*/`)
)

type tsReasonCase struct {
	code string
	keys []string
}

func TestTheComposerWordsEveryRefusalTheSendEngineGives(t *testing.T) {
	t.Parallel()
	cases, generic := tsSendPermissionCases(t)
	inGo := commsauthz.RefusalCodes()
	inTS := make([]string, 0, len(cases))
	for _, c := range cases {
		inTS = append(inTS, c.code)
	}
	for _, code := range inGo {
		if !slices.Contains(inTS, code) {
			t.Errorf("the send engine refuses with %q and %s has no case for it, so the composer shows the generic refusal. Add a case with its own sentence there", code, frontendSendPermissionReasons)
		}
	}
	for _, code := range inTS {
		if !slices.Contains(inGo, code) {
			t.Errorf("%s: %s has a case for %q, which %s does not list. Remove the case, or add the code there", frontendSendPermissionReasons, sendPermissionReasonMapper, code, serverRefusalCodes)
		}
	}
	checkReasonSentences(t, cases, generic)
}

// checkReasonSentences holds each code to a sentence of its own that every catalog carries.
// A shared or generic sentence names neither the cause nor the fix.
func checkReasonSentences(t *testing.T, cases []tsReasonCase, generic string) {
	t.Helper()
	codesByKey := map[string][]string{}
	for _, c := range cases {
		for _, key := range c.keys {
			if key == generic {
				t.Errorf("%s: %s sends %q to the generic %q. Give it a sentence of its own", frontendSendPermissionReasons, sendPermissionReasonMapper, c.code, generic)
			}
			codesByKey[key] = append(codesByKey[key], c.code)
		}
	}
	for key, codes := range codesByKey {
		if len(codes) > 1 {
			t.Errorf("%s: %s gives %v one sentence, %q. Each refusal needs its own cause and fix", frontendSendPermissionReasons, sendPermissionReasonMapper, codes, key)
		}
	}
	for _, catalog := range frontendCatalogs {
		keys := tsCatalogKeys(t, catalog)
		for key := range codesByKey {
			if !keys[key] {
				t.Errorf("frontend/src/i18n/%s has no %q, which %s returns. Add the sentence there", catalog, key, sendPermissionReasonMapper)
			}
		}
		if !keys[generic] {
			t.Errorf("frontend/src/i18n/%s has no %q, the generic refusal %s falls back to", catalog, generic, sendPermissionReasonMapper)
		}
	}
}

func tsCatalogKeys(t *testing.T, catalog string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(frontendI18nDir, catalog))
	if err != nil {
		t.Fatalf("reading the %s catalog: %v", catalog, err)
	}
	keys := map[string]bool{}
	for _, m := range tsCatalogKey.FindAllStringSubmatch(tsBlockComment.ReplaceAllString(string(raw), " "), -1) {
		keys[m[1]] = true
	}
	if len(keys) == 0 {
		t.Fatalf("no message keys parsed out of frontend/src/i18n/%s. A gate that reads nothing agrees with everything", catalog)
	}
	return keys
}

// tsSendPermissionCases reads each case of the mapper's switch with the key it returns,
// and the default's key, comments removed.
func tsSendPermissionCases(t *testing.T) (cases []tsReasonCase, generic string) {
	t.Helper()
	body := tsMapperBody(t)
	labels := tsCaseLabel.FindAllStringSubmatchIndex(body, -1)
	read := 0
	for i, at := range labels {
		next := len(body)
		if i+1 < len(labels) {
			next = labels[i+1][0]
		}
		keys := tsClauseKeys(t, body[at[1]:next], body[at[0]:at[1]])
		if at[8] >= 0 {
			generic = keys[0]
			continue
		}
		read++
		cases = append(cases, tsReasonCase{code: tsGroup(body, at, 1) + tsGroup(body, at, 2) + tsGroup(body, at, 3), keys: keys})
	}
	if keywords := len(tsCaseKeyword.FindAllString(body, -1)); keywords != read {
		t.Fatalf("%s: %s has %d case statements and %d name one string literal. Write each case as a quoted code", frontendSendPermissionReasons, sendPermissionReasonMapper, keywords, read)
	}
	if read == 0 || generic == "" {
		t.Fatalf("%s: read %d cases and default key %q out of %s. A gate that reads nothing agrees with everything", frontendSendPermissionReasons, read, generic, sendPermissionReasonMapper)
	}
	return cases, generic
}

func tsMapperBody(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(frontendSendPermissionReasons)
	if err != nil {
		t.Fatalf("reading the frontend refusal mapper: %v", err)
	}
	source := tsComment.ReplaceAllString(string(raw), " ")
	declarations := tsMapperDeclaration.FindAllStringIndex(source, -1)
	if len(declarations) != 1 {
		t.Fatalf("%s declares function %s %d times, want once. After a rename this gate reads nothing; rename it here too", frontendSendPermissionReasons, sendPermissionReasonMapper, len(declarations))
	}
	body := source[declarations[0][1]:]
	end := indexAfter(body, "\n}\n")
	if end < 0 {
		t.Fatalf("%s: function %s is unterminated", frontendSendPermissionReasons, sendPermissionReasonMapper)
	}
	return body[:end]
}

// tsClauseKeys reads the quoted keys a case clause returns: one key, or one per branch of a ternary.
// A fall-through, a computed key or a branch without a quoted key fails.
func tsClauseKeys(t *testing.T, clause, label string) []string {
	t.Helper()
	statements := tsReturnStatement.FindAllStringSubmatch(clause, -1)
	if len(statements) != 1 || len(tsReturnKeyword.FindAllString(clause, -1)) != 1 {
		t.Fatalf("%s: %s `%s` must hold exactly one return statement; found %d", frontendSendPermissionReasons, sendPermissionReasonMapper, label, len(statements))
	}
	expression := statements[0][1]
	quoted := tsReturnedLiteral.FindAllStringSubmatch(expression, -1)
	if branches := strings.Count(expression, "?") + 1; len(quoted) != branches {
		t.Fatalf("%s: %s `%s` returns %d quoted keys over %d branches. Return a quoted message key from each branch", frontendSendPermissionReasons, sendPermissionReasonMapper, label, len(quoted), branches)
	}
	keys := make([]string, 0, len(quoted))
	for _, m := range quoted {
		keys = append(keys, m[1]+m[2]+m[3])
	}
	return keys
}

func tsGroup(source string, at []int, group int) string {
	if at[2*group] < 0 {
		return ""
	}
	return source[at[2*group]:at[2*group+1]]
}
