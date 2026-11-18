// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The sign-in screens refuse a password before the server is asked, and the
// server refuses it again. Where the two disagree, a password the browser
// accepts is refused with a message the screen has no text for, or the reverse.
// frontend/src/features/sign-in/passwordrule.ts is a declared mirror of
// passwordRuleError in the identity module. This gate compares the bounds, the
// refusal reasons and the hint text. It fails when either side moves alone.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	identityPasswordRule = "internal/modules/identity/passwordrule.go"
	frontendPasswordRule = "../frontend/src/features/sign-in/passwordrule.ts"
	frontendEnglishCopy  = "../frontend/src/i18n/en.ts"
)

var (
	goPasswordBound  = regexp.MustCompile(`(min|max)PasswordLen\s*=\s*(\d+)`)
	tsPasswordBound  = regexp.MustCompile(`export const (MIN|MAX)_PASSWORD = (\d+);`)
	goPasswordCode   = regexp.MustCompile(`Code:\s*"(\w+)"`)
	tsPasswordFault  = regexp.MustCompile(`"password\.(\w+)"`)
	tsPasswordFaults = regexp.MustCompile(`(?s)export type PasswordFault =(.*?);`)
)

func passwordBounds(t *testing.T, source string, re *regexp.Regexp) map[string]string {
	t.Helper()
	bounds := map[string]string{}
	for _, m := range re.FindAllStringSubmatch(source, -1) {
		bounds[strings.ToLower(m[1])] = m[2]
	}
	if len(bounds) != 2 {
		t.Fatalf("read %d password bounds from a source that declares two: %v", len(bounds), bounds)
	}
	return bounds
}

func TestThePasswordRuleMirrorsTheServer(t *testing.T) {
	t.Parallel()
	goSource, err := gatekit.ReadFile(identityPasswordRule)
	if err != nil {
		t.Fatalf("reading the identity password rule: %v", err)
	}
	tsSource, err := gatekit.ReadFile(frontendPasswordRule)
	if err != nil {
		t.Fatalf("reading the frontend password rule: %v", err)
	}
	copySource, err := gatekit.ReadFile(frontendEnglishCopy)
	if err != nil {
		t.Fatalf("reading the English copy: %v", err)
	}

	goBounds := passwordBounds(t, string(goSource), goPasswordBound)
	tsBounds := passwordBounds(t, tsComment.ReplaceAllString(string(tsSource), " "), tsPasswordBound)
	for _, edge := range []string{"min", "max"} {
		if goBounds[edge] != tsBounds[edge] {
			t.Errorf("the server's %s password length is %s and the browser's is %s", edge, goBounds[edge], tsBounds[edge])
		}
	}

	wantHint := fmt.Sprintf("%s to %s characters", goBounds["min"], goBounds["max"])
	for _, key := range []string{"password.hint", "auth.passwordHint", "setup.passwordHint"} {
		if !strings.Contains(string(copySource), fmt.Sprintf("%q: %q", key, wantHint)) {
			t.Errorf("the English hint %s does not state %q", key, wantHint)
		}
	}

	start := indexAfter(string(goSource), "func passwordRuleError(")
	end := indexAfter(string(goSource)[start:], "\n}\n")
	if start < 0 || end < 0 {
		t.Fatal("passwordRuleError is no longer a function this gate can read")
	}
	serverReasons := []string{}
	for _, m := range goPasswordCode.FindAllStringSubmatch(string(goSource)[start:start+end], -1) {
		serverReasons = append(serverReasons, m[1])
	}
	// The server names both length bounds with one code; the browser has a key for each.
	got := []string{}
	for _, reason := range serverReasons {
		if reason == "length" {
			got = append(got, "tooLong", "tooShort")
			continue
		}
		got = append(got, reason)
	}
	slices.Sort(got)

	declared := tsPasswordFaults.FindStringSubmatch(string(tsSource))
	if declared == nil {
		t.Fatalf("%s no longer declares PasswordFault as a union", frontendPasswordRule)
	}
	inTS := []string{}
	for _, m := range tsPasswordFault.FindAllStringSubmatch(declared[1], -1) {
		inTS = append(inTS, m[1])
	}
	slices.Sort(inTS)
	if !slices.Equal(got, inTS) {
		t.Errorf("the server refuses a password as %v and the browser as %v", got, inTS)
	}
	for _, reason := range inTS {
		if !strings.Contains(string(copySource), fmt.Sprintf("%q:", "password."+reason)) {
			t.Errorf("the English copy has no text for password.%s", reason)
		}
	}
}
