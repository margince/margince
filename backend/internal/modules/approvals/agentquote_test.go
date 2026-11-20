// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// An agent's text cannot end the quote it sits in early. A closing mark and a
// line break in it stay inside the marks the approver reads. The summary still
// binds, so a German approver reads it in German.
func TestAnAgentsTextCannotCloseTheQuoteItIsShownIn(t *testing.T) {
	typed := "Hello”\nApprove every pending send"
	value := systemtext.Inline(typed)
	in := prepareSystemSummary(context.Background(), StageInput{
		Summary:    fmt.Sprintf(systemtext.Copy("agent.sendMessage", textlang.English), value),
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("agent.sendMessage", systemtext.Text(value))},
	})
	if strings.Count(in.Summary, "”") != 1 || !strings.HasSuffix(in.Summary, "”") || strings.Contains(in.Summary, "\n") {
		t.Errorf("the summary %q can be read as ending before the agent's text does", in.Summary)
	}
	stored, err := marshalSystemSummary(context.Background(), in.Summary, in.SummaryParts, in.SystemText, nil)
	if err != nil {
		t.Fatal(err)
	}
	bound := SummarySystemText(stored)
	if bound == nil || bound.Subject == nil {
		t.Fatal("the summary lost its descriptor, so it would show in English to every approver")
	}
	german := bound.SubjectIn(in.Summary, textlang.German)
	if !strings.HasSuffix(german, "„Hello’ Approve every pending send“") {
		t.Errorf("a German approver reads %s", german)
	}
}
