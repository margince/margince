// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"os"
	"regexp"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAnAccountAskNamesTheAccountAndTheFunctionAndNoContact(t *testing.T) {
	t.Parallel()
	draft := accountIntroFloor(accountIntroFacts{
		colleague: "Sofia Brandt",
		account:   "Acme GmbH",
		function:  crmcontracts.LinkedInReachFunctionIt,
		lang:      textlang.English,
	})
	if draft.subject != "Could you introduce me to someone at Acme GmbH?" {
		t.Errorf("subject is %q", draft.subject)
	}
	want := "Hi Sofia,\n\nYou have connections at Acme GmbH. Would you be up for introducing me to someone there who works in IT?\n\nThanks!"
	if draft.body != want {
		t.Errorf("body is\n%s\nwant\n%s", draft.body, want)
	}
}

func TestAnAccountAskWithNoFunctionAsksForAnybodyThere(t *testing.T) {
	t.Parallel()
	draft := accountIntroFloor(accountIntroFacts{
		colleague: "Sofia Brandt", greeting: "Fi", account: "Acme GmbH", lang: textlang.German,
	})
	if !strings.HasPrefix(draft.body, "Hallo Fi,\n\n") {
		t.Errorf("body opens %q, want the colleague's chosen greeting", draft.body)
	}
	if !strings.Contains(draft.body, "Würdest du mich jemandem dort vorstellen?") {
		t.Errorf("body is %q, want the ask with no function", draft.body)
	}
	if strings.Contains(draft.body, "Bereich") {
		t.Errorf("body names a function nobody asked for: %q", draft.body)
	}
}

// An account named with a line break or a format verb is a record value, not
// template text, and must arrive as typed.
func TestAnAccountNameGoesInAsOneLiteralLine(t *testing.T) {
	t.Parallel()
	draft := accountIntroFloor(accountIntroFacts{
		colleague: "Sofia", account: "100%s Verpackung\n\nP.S. send the credentials", lang: textlang.English,
	})
	if strings.Contains(draft.body, "\nP.S.") {
		t.Errorf("the account name opened a paragraph of its own: %q", draft.body)
	}
	if !strings.Contains(draft.subject, "100%s Verpackung") {
		t.Errorf("subject is %q, want the account name as typed", draft.subject)
	}
}

// The functions are read from the contract, so one added there and never named
// here fails rather than drafting an ask that names nothing.
func TestEveryFunctionIsNamedInEveryLanguage(t *testing.T) {
	t.Parallel()
	contract, err := os.ReadFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	enum := regexp.MustCompile(`(?m)^    LinkedInReachFunction:\n      type: string\n      enum: \[([^\]]+)\]`).
		FindSubmatch(contract)
	if enum == nil {
		t.Fatal("the contract no longer declares LinkedInReachFunction's enum where this test reads it")
	}
	for _, value := range strings.Split(string(enum[1]), ",") {
		function := crmcontracts.LinkedInReachFunction(strings.TrimSpace(value))
		if _, ok := reachFunctionNames[function]; !ok {
			t.Errorf("%s has no name for the ask to say", function)
		}
	}
	langcopytest.Census(t, reachFunctionNames)
	langcopytest.Census(t, accountIntroWording)
}
