// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The command tree, asked the way an operator asks it.
//
// The obligation these hold is that the catalog, the dispatch and the help
// cannot come apart: every verb the tool advertises answers for itself, and a
// verb nobody serves sends the reader to the list rather than to a sentence.

import (
	"context"
	"flag"
	"strings"
	"testing"
)

// Every advertised verb answers --help, with its own name and its own flags. A
// verb in the catalog that no parser serves would fail here rather than at the
// moment an operator typed it.
func TestEveryAdvertisedVerbAnswersForItself(t *testing.T) {
	for _, verb := range aiTaskVerbs() {
		t.Run(verb.name, func(t *testing.T) {
			var out strings.Builder
			if err := runAITaskProbe(context.Background(), []string{verb.name, "--help"}, &out); err != nil {
				t.Fatalf("`aitask %s --help`: %v", verb.name, err)
			}
			help := out.String()
			if !strings.Contains(help, verb.summary) {
				t.Errorf("`aitask %s --help` does not say what the verb is for:\n%s", verb.name, help)
			}
			if verb.flags == nil {
				return
			}
			// Derived rather than listed: the help renders the parser's own flag
			// set, so naming one flag per verb here would be a second list to
			// keep true. Asking the set what it holds is asking the same source.
			verb.flags().VisitAll(func(f *flag.Flag) {
				if !strings.Contains(help, "-"+f.Name) {
					t.Errorf("`aitask %s --help` omits the -%s flag its parser accepts", verb.name, f.Name)
				}
			})
		})
	}
}

// `aitask help` and a bare `aitask` both print the tree, because an operator who
// types the tool's name with nothing after it is asking the same question.
func TestTheTreeIsPrintedForHelpAndForNothingAtAll(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}} {
		var out strings.Builder
		if err := runAITaskProbe(context.Background(), args, &out); err != nil {
			t.Fatalf("`aitask %v`: %v", args, err)
		}
		for _, verb := range aiTaskVerbs() {
			if !strings.Contains(out.String(), verb.name) {
				t.Errorf("`aitask %v` does not name the %s verb:\n%s", args, verb.name, out.String())
			}
		}
	}
}

// A mistyped verb gets the list AND a non-zero exit. The list, because the
// operator needs it more than the sentence; the exit, because a script must not
// read a typo as work done.
func TestAVerbNobodyServesPrintsTheTreeAndStillRefuses(t *testing.T) {
	var out strings.Builder
	err := runAITaskProbe(context.Background(), []string{"probe"}, &out)
	if err == nil {
		t.Fatal("an unknown verb was accepted")
	}
	if !strings.Contains(err.Error(), "unknown verb") {
		t.Errorf("refusal = %q", err)
	}
	if !strings.Contains(out.String(), verbRetrieve) {
		t.Errorf("the tree was not printed alongside the refusal:\n%s", out.String())
	}
}

// `aitask help <verb>` is the same answer as `aitask <verb> --help`, which is
// what makes the tree navigable from either habit.
func TestHelpForOneVerbMatchesThatVerbsOwnHelp(t *testing.T) {
	var viaHelp, viaFlag strings.Builder
	if err := runAITaskProbe(context.Background(), []string{"help", verbRetrieve}, &viaHelp); err != nil {
		t.Fatalf("`aitask help retrieve`: %v", err)
	}
	if err := runAITaskProbe(context.Background(), []string{verbRetrieve, "-h"}, &viaFlag); err != nil {
		t.Fatalf("`aitask retrieve -h`: %v", err)
	}
	if viaHelp.String() != viaFlag.String() {
		t.Errorf("the two spellings print different help:\n--- help retrieve ---\n%s\n--- retrieve -h ---\n%s",
			viaHelp.String(), viaFlag.String())
	}
}
