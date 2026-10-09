// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"regexp"
	"testing"
)

// The skill tells an agent which passport permission a call needs, in the
// words the human ticked on the Agent passports card. permissionNames mirrors
// the card's strings, and this compares the two both ways.

const frontendMessages = "../../../frontend/src/i18n/en.ts"

var scopeMessage = regexp.MustCompile(`"passport\.scope\.([a-z_]+)":\s*"([^"]*)"`)

func TestPermissionNamesMirrorTheCard(t *testing.T) {
	source, err := os.ReadFile(frontendMessages)
	if err != nil {
		t.Fatalf("reading the card's strings: %v", err)
	}
	onCard := map[string]string{}
	for _, m := range scopeMessage.FindAllStringSubmatch(string(source), -1) {
		onCard[m[1]] = m[2]
	}
	if len(onCard) == 0 {
		t.Fatalf("%s holds no passport.scope.* string, so this reads a shape that is gone", frontendMessages)
	}
	for scope, name := range onCard {
		if permissionNames[scope] != name {
			t.Errorf("the card names scope %s %q, and the skill names it %q", scope, name, permissionNames[scope])
		}
	}
	for scope, name := range permissionNames {
		if _, ok := onCard[scope]; !ok {
			t.Errorf("the skill names scope %s %q, which the card has no string for", scope, name)
		}
	}
}
