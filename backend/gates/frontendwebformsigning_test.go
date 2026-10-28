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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

const frontendWebFormKinds = "../frontend/src/features/settings/webformkinds.ts"

// The setup dialog shows a minted secret and offers to rotate it only for the
// senders whose secret the server mints. A drift either way strands a form:
// a secret never shown, or a rotation the server refuses.
func TestTheSetupDialogMintsTheSecretsTheServerMints(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendWebFormKinds)
	if err != nil {
		t.Fatalf("reading the web form kinds: %v", err)
	}
	senders := quotedListOf(t, source, "WEB_FORM_SENDERS")
	minted := map[string]bool{}
	for _, sender := range quotedListOf(t, source, "MINTED_SECRET_SENDERS") {
		minted[sender] = true
	}
	for _, sender := range senders {
		if !crmcontracts.WebFormSender(sender).Valid() {
			t.Errorf("the dialog offers %q, which the contract does not name", sender)
		}
		if want := webforms.MintsSecret(sender, nil); minted[sender] != want {
			t.Errorf("%q: the dialog says minted %v, the server %v", sender, minted[sender], want)
		}
		delete(minted, sender)
	}
	for sender := range minted {
		t.Errorf("%q is minted in the dialog but offered nowhere", sender)
	}
}

// quotedListOf reads `const NAME: ... = ["a", "b"];` from TypeScript source,
// and fails on an empty or missing list, which would compare nothing.
func quotedListOf(t *testing.T, source []byte, name string) []string {
	t.Helper()
	body := regexp.MustCompile(`(?s)const ` + name + `[^=]*= \[(.*?)\];`).FindSubmatch(source)
	if body == nil {
		t.Fatalf("%s declares no %s, so this gate cannot compare it", frontendWebFormKinds, name)
	}
	var out []string
	for _, quoted := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(body[1], -1) {
		out = append(out, string(quoted[1]))
	}
	if len(out) == 0 {
		t.Fatalf("%s lists no senders in %s", frontendWebFormKinds, name)
	}
	return out
}
