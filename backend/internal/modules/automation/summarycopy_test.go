// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// Every shipped language writes the staging summaries in its own words, with
// every formatting hole the English sentence has: a dropped %s stops naming the
// addressee a human is asked to approve a reply to.
func TestEveryShippedLanguageWritesItsOwnAutomationSummaries(t *testing.T) {
	english, ok := summaryByLang[textlang.English]
	if !ok {
		t.Fatal("English has no summary set, and it is the fallback for everything else")
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := summaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			for _, field := range []struct {
				name, got, en string
				holes         int
			}{
				{"stagedAction", said.stagedAction, english.stagedAction, 3},
				{"heldDraft", said.heldDraft, english.heldDraft, 1},
			} {
				if field.got == "" {
					t.Errorf("%s is empty, and a blank summary reaches the inbox as nothing at all", field.name)
					continue
				}
				if got := strings.Count(field.got, "%s"); got != field.holes {
					t.Errorf("%s carries %d %%s holes, want %d: %q", field.name, got, field.holes, field.got)
				}
				if got := strings.Count(field.got, "%"); got != field.holes {
					t.Errorf("%s carries a formatting verb other than %%s: %q", field.name, field.got)
				}
				if lang != textlang.English && field.got == field.en {
					t.Errorf("%s is the English sentence verbatim; the entry exists and the reader still gets English",
						field.name)
				}
			}
		})
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row an admin can edit by hand.
func TestAnUnknownLanguageStagesInEnglish(t *testing.T) {
	unshipped := func(context.Context) textlang.Lang { return textlang.Lang("kl") }
	if got := summaryIn(context.Background(), unshipped); got != summaryByLang[textlang.English] {
		t.Errorf("an unshipped language answered %+v, want the English set", got)
	}
}

func germanInstallation(context.Context) textlang.Lang { return textlang.German }

// A staged action's inbox line is written in the language the injected
// resolver answers, not in the English the code is written in.
func TestAStagedActionSummaryFollowsTheInjectedLanguage(t *testing.T) {
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	target := datasource.EntityRef{Type: datasource.EntityDeal, ID: ids.NewV7()}
	action := workflow.Action{Kind: workflow.ActionEmitFlowEvent, Target: target, Args: json.RawMessage(`{}`)}

	_, err := ApplyActions(context.Background(), Executors{Approvals: approvals, Language: germanInstallation},
		workflow.Effect{Actions: []workflow.Action{action}})
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) || len(approvals.calls) != 1 {
		t.Fatalf("ApplyActions err = %v with %d stagings, want exactly one staged approval", err, len(approvals.calls))
	}
	want := "Eine Automatisierung möchte emit_flow_event für deal " + target.ID.String() + " ausführen"
	if got := approvals.calls[0].Summary; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// A held draft names its addressee in the installation's language.
func TestAHeldDraftSummaryFollowsTheInjectedLanguage(t *testing.T) {
	approvals := &fakeApprovals{id: ids.New[ids.ApprovalKind]()}
	action := workflow.Action{
		Kind:   workflow.ActionDraftEmail,
		Target: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.NewV7()},
		Args:   json.RawMessage(`{"intent":"nudge","consent_purpose":"business_correspondence"}`),
	}

	_, err := ApplyActions(ownedFiring(),
		Executors{Comms: &fakeComms{subject: "Re: hallo", body: "b"}, Approvals: approvals, Language: germanInstallation},
		workflow.Effect{Actions: []workflow.Action{action}})
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) || len(approvals.calls) != 1 {
		t.Fatalf("ApplyActions err = %v with %d stagings, want exactly one staged draft", err, len(approvals.calls))
	}
	want := "Eine Automatisierung hat eine Antwort an " + replyAddressDefault +
		" entworfen. Lies sie, bevor sie versendet wird."
	if got := approvals.calls[0].Summary; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}
