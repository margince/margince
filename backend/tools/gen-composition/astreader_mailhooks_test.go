// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"slices"
	"testing"
)

// mailHooksUnitSource is a unit declaring the given MailHooks literal.
func mailHooksUnitSource(hooks string) string {
	return `package x

import "github.com/margince/margince/backend/pkg/extension"

func New() extension.Extension {
	return extension.Extension{
		Name:        "x",
		Version:     "0.1.0",
		Description: "A unit composed by a test.",
		MailHooks:   ` + hooks + `,
	}
}
`
}

func derivedMailHooks(t *testing.T, hooks string) *declaredMailHooks {
	t.Helper()
	derived, err := deriveSynthetic(t, "x", mailHooksUnitSource(hooks))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		MailHooks *declaredMailHooks `json:"mail_hooks"`
	}
	if err := json.Unmarshal(derived, &m); err != nil {
		t.Fatal(err)
	}
	return m.MailHooks
}

// The manifest tells an operator that the unit reads captured mail, and how much
// of it, before the unit runs.
func TestMailHooksDeriveIntoTheManifest(t *testing.T) {
	got := derivedMailHooks(t, `extension.MailHooks{Parties: nil, Deal: decideDeal, Reasons: []string{"po_number", "credit_hold"}}`)
	if got == nil {
		t.Fatal("the manifest does not name the unit's mail hooks")
	}
	if got.Parties || !got.Deal {
		t.Errorf("parties = %v, deal = %v, want only the deal hook", got.Parties, got.Deal)
	}
	if !slices.Equal(got.Reasons, []string{"credit_hold", "po_number"}) {
		t.Errorf("reasons = %v, want them sorted", got.Reasons)
	}
	if !slices.Equal(got.Reads, []string{"subject", "addresses", "body"}) || got.BodyCharacters != 8000 {
		t.Errorf("reads = %v of %d characters, want subject, addresses and 8000 characters of body", got.Reads, got.BodyCharacters)
	}
}

func TestAUnitWithoutMailHooksHasNoManifestEntry(t *testing.T) {
	if got := derivedMailHooks(t, `extension.MailHooks{}`); got != nil {
		t.Errorf("a unit with no hook was recorded as reading mail: %+v", got)
	}
}

// Generation refuses what MailHooks.Validate refuses at boot.
func TestMailHooksTheBootWouldRefuseAreRefusedAtGeneration(t *testing.T) {
	for name, hooks := range map[string]string{
		"no reason":         `extension.MailHooks{Deal: decideDeal}`,
		"a malformed key":   `extension.MailHooks{Deal: decideDeal, Reasons: []string{"PO Number"}}`,
		"a repeated key":    `extension.MailHooks{Deal: decideDeal, Reasons: []string{"po", "po"}}`,
		"a computed reason": `extension.MailHooks{Deal: decideDeal, Reasons: reasons}`,
		"an unknown field":  `extension.MailHooks{Deal: decideDeal, Reasons: []string{"po"}, Limit: 3}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := deriveSynthetic(t, "x", mailHooksUnitSource(hooks)); err == nil {
				t.Fatal("generation accepted mail hooks the boot refuses")
			}
		})
	}
}
