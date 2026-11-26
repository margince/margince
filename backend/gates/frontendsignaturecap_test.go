// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The personal HTML signature cap is one rule with three spellings. The Go
// constant refuses, the contract's maxLength is what clients read, and the
// dialog's counter warns. Raise the Go cap alone, and the counter turns red for
// markup the server saves. Lower it alone, and Save fails under a counter that
// said nothing. The contract side is derived, so a fourth html field on a
// signature schema joins the check without an edit here.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/signature"
)

const frontendSignatureEditor = "../frontend/src/features/settings/settingssignatureeditor.tsx"

var (
	tsSignatureCap    = regexp.MustCompile(`(?m)^export const SIGNATURE_HTML_MAX_CHARS = ([0-9_]+);`)
	tsSignatureNumber = regexp.MustCompile(`\b[0-9][0-9_]*\b`)
)

// signatureHTMLOutputs are the html fields the server writes rather than reads.
// They carry no cap, because a preview's markup grows with its drawn images.
var signatureHTMLOutputs = map[string]bool{"EmailSignOff": true}

func TestTheSignatureHTMLCapAgreesInGoTheContractAndTheDialog(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendSignatureEditor)
	if err != nil {
		t.Fatalf("reading the signature editor: %v", err)
	}
	match := tsSignatureCap.FindStringSubmatch(string(source))
	if match == nil {
		t.Fatalf("%s no longer exports SIGNATURE_HTML_MAX_CHARS as a number — this gate is reading a shape that is gone", frontendSignatureEditor)
	}
	inTS, err := strconv.Atoi(strings.ReplaceAll(match[1], "_", ""))
	if err != nil {
		t.Fatalf("%s: %q is not a number", frontendSignatureEditor, match[1])
	}
	if inTS != signature.MaxHTMLRunes {
		t.Errorf("the dialog counts to %d and the server refuses past %d", inTS, signature.MaxHTMLRunes)
	}

	rest := strings.Replace(string(source), match[0], "", 1)
	if uses := strings.Count(rest, "SIGNATURE_HTML_MAX_CHARS"); uses < 2 {
		t.Errorf("the counter reads SIGNATURE_HTML_MAX_CHARS %d times, want its threshold and its danger tone", uses)
	}
	for _, literal := range tsSignatureNumber.FindAllString(tsComment.ReplaceAllString(rest, " "), -1) {
		if n, err := strconv.Atoi(strings.ReplaceAll(literal, "_", "")); err == nil && n >= 1000 {
			t.Errorf("%s types %s, a cap that does not read SIGNATURE_HTML_MAX_CHARS", frontendSignatureEditor, literal)
		}
	}

	inputs, outputs := 0, map[string]bool{}
	for name, schema := range descendContract(t, loadContractDocument(t), "components", "schemas") {
		node, isObject := schema.(map[string]any)
		properties, hasProperties := node["properties"].(map[string]any)
		html, hasHTML := properties["html"].(map[string]any)
		if !strings.Contains(name, "Signature") && !strings.Contains(name, "SignOff") || !isObject || !hasProperties || !hasHTML {
			continue
		}
		if signatureHTMLOutputs[name] {
			outputs[name] = true
			continue
		}
		inputs++
		if got, ok := html["maxLength"].(int); !ok || got != signature.MaxHTMLRunes {
			t.Errorf("%s.html has maxLength %v in crm.yaml and the server refuses past %d", name, html["maxLength"], signature.MaxHTMLRunes)
		}
	}
	for name := range signatureHTMLOutputs {
		if !outputs[name] {
			t.Errorf("%s is listed as a signature html output and has no html field in crm.yaml", name)
		}
	}
	// EmailSignature, EmailSignatureDraft and SaveEmailSignatureRequest today.
	if inputs < 3 {
		t.Fatalf("found %d signature html fields in crm.yaml, want at least 3 — a gate that reads fewer agrees with more", inputs)
	}
}
