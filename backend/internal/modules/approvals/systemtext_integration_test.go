// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestStagedSystemWordsFollowTheReaderWithoutExposingTheirDescriptor(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	in := e.agentCall(e.seedCompany(t))
	in.Summary = systemtext.Literal("approval.unsummarized")
	in.SystemText = &systemtext.Message{Subject: systemtext.Sentence("approval.unsummarized")}
	in.EffectSystemText = systemtext.ForWrite(e.asDecider(), &systemtext.Message{Subject: systemtext.Prose(systemtext.Generated, textlang.English)}, "Generated task instruction", "")
	staged, _, err := e.svc.StageAgentCall(e.asPassport(passport), in)
	if err != nil {
		t.Fatal(err)
	}
	row, err := e.svc.Get(e.asDecider(), staged)
	if err != nil || EffectSystemText(row.SummaryParts) == nil {
		t.Fatalf("private effect provenance missing: %v", err)
	}
	for _, lang := range textlang.Shipped {
		got, err := e.svc.GetWire(systemtext.WithLocale(e.asDecider(), lang), staged)
		if err != nil {
			t.Fatal(err)
		}
		if got.SystemDisplay == nil || got.SystemDisplay.Subject == nil || *got.SystemDisplay.Subject != systemtext.Copy("approval.unsummarized", lang) {
			t.Fatalf("%s missing display: %+v", lang, got.SystemDisplay)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "_effect_text") || strings.Contains(string(encoded), "schema_version") {
			t.Fatal("private descriptor escaped on approval wire")
		}
		if got.Summary == nil || *got.Summary != in.Summary {
			t.Fatal("reader changed canonical proposal")
		}
	}
	replay, _, err := e.svc.StageAgentCall(e.asPassport(passport), in)
	if err != nil || replay != staged {
		t.Fatalf("replay created another approval: %v %v", replay, err)
	}
}

func TestLongImmutableProposalFactsRetainTheFullPreviewAndGenericDisplay(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	in := e.agentCall(e.seedCompany(t))
	fact := strings.Repeat("x", 600)
	component := systemtext.Sentence("approval.enrichmentOf", systemtext.Text(fact))
	in.Summary = systemtext.Canonical(component, textlang.English)
	in.SystemText = &systemtext.Message{Subject: component}
	in.SummaryParts = &crmcontracts.ApprovalSummaryParts{Sentence: in.Summary, Fields: []crmcontracts.ApprovalSummaryField{}}
	staged, _, err := e.svc.StageAgentCall(e.asPassport(passport), in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.GetWire(systemtext.WithLocale(e.asDecider(), textlang.German), staged)
	if err != nil {
		t.Fatal(err)
	}
	if got.SystemDisplay == nil || got.SystemDisplay.Subject == nil || *got.SystemDisplay.Subject != systemtext.Copy("approval.unsummarized", textlang.German) {
		t.Fatalf("missing generic localized caption: %+v", got.SystemDisplay)
	}
	row, err := e.svc.Get(e.asDecider(), staged)
	if err != nil {
		t.Fatal(err)
	}
	var stored, proposed map[string]string
	if err := json.Unmarshal(row.ProposedChange, &stored); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(in.ProposedChange, &proposed); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(stored, proposed) {
		t.Fatal("display limit changed proposed effect")
	}
}
