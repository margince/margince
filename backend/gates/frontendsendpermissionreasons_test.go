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
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

var frontendSendPermissionReasons = filepath.Join(repoRoot, "frontend", "src", "format", "sendpermissionreason.ts")

const (
	sendPermissionReasonMapper = "sendPermissionReasonKey"
	serverRefusalCodes         = "commsauthz.RefusalCodes in backend/internal/shared/ports/commsauthz/refusalcodes.go"
)

// tsCaseKeyword counts the case statements, and tsStringCase the ones this gate can read.
// A case it cannot read fails the gate rather than shrinking the census.
var (
	tsMapperDeclaration = regexp.MustCompile(`\bfunction\s+` + sendPermissionReasonMapper + `\s*\(`)
	tsCaseKeyword       = regexp.MustCompile(`\bcase\b`)
	tsStringCase        = regexp.MustCompile("\\bcase\\s+(?:\"([^\"]*)\"|'([^']*)'|`([^`]*)`)\\s*:")
)

func TestTheComposerWordsEveryRefusalTheSendEngineGives(t *testing.T) {
	t.Parallel()
	inTS := tsSendPermissionCases(t)
	inGo := commsauthz.RefusalCodes()
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
}

// tsSendPermissionCases reads the code each case of the mapper's switch names, comments removed.
func tsSendPermissionCases(t *testing.T) []string {
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
	body = body[:end]
	matches := tsStringCase.FindAllStringSubmatch(body, -1)
	if keywords := len(tsCaseKeyword.FindAllString(body, -1)); keywords != len(matches) {
		t.Fatalf("%s: %s has %d case statements and %d name one string literal. Write each case as a quoted code", frontendSendPermissionReasons, sendPermissionReasonMapper, keywords, len(matches))
	}
	if len(matches) == 0 {
		t.Fatalf("%s: no case parsed out of %s. A gate that reads nothing agrees with everything", frontendSendPermissionReasons, sendPermissionReasonMapper)
	}
	cases := make([]string, 0, len(matches))
	for _, m := range matches {
		cases = append(cases, m[1]+m[2]+m[3])
	}
	return cases
}
