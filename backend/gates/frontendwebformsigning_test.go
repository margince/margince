// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The setup dialog hands a website's developer the code that signs a server
// form's posts. If its signing scope or endpoint name drift from what the
// intake verifies, every pasted copy signs material the server never accepts.

import (
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/webforms"
	"github.com/margince/margince/backend/pkg/extension"
)

const frontendWebFormSnippet = "../frontend/src/features/settings/webformsnippet.ts"

func TestTheWebFormSnippetSignsWhatTheIntakeVerifies(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendWebFormSnippet)
	if err != nil {
		t.Fatalf("reading the web form snippet: %v", err)
	}
	for name, want := range map[string]string{
		"SIGNING_SCOPE": string(extension.ScopeInbound),
		"SIGNING_SLUG":  webforms.SigningSlug,
	} {
		match := regexp.MustCompile(`const ` + name + ` = "([^"]*)";`).FindSubmatch(source)
		if match == nil {
			t.Errorf("%s declares no %s, so this gate cannot compare it", frontendWebFormSnippet, name)
			continue
		}
		if got := string(match[1]); got != want {
			t.Errorf("%s is %q in the snippet and %q on the server", name, got, want)
		}
	}
}
