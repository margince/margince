// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package workflow_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// A staged error must stay matchable by errors.As across module boundaries:
// the Surface-B runner suspends on the id, so a caller that only sees the
// unwrapped sentinel has lost the approval it needs to resume.
func TestStagedApprovalErrorCarriesTheApprovalIDThroughErrorsAs(t *testing.T) {
	id := ids.NewV7()
	var err error = &workflow.StagedApprovalError{ApprovalID: ids.ApprovalID{UUID: id}}

	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatal("errors.As did not match StagedApprovalError")
	}
	if staged.ApprovalID.UUID != id {
		t.Errorf("ApprovalID = %v, want %v", staged.ApprovalID.UUID, id)
	}
	if !errors.Is(err, apperrors.ErrRequiresApproval) {
		t.Error("staged error must unwrap to ErrRequiresApproval")
	}
}

// The caller that wrote the arguments is the one relaying the wait to a contact,
// so the answer has to carry what was staged and not only that something was.
func TestAStagedAnswerRepeatsWhatWasStaged(t *testing.T) {
	const summary = "Update contact Ada Lovelace: overwrite human-edited job_title"
	id := ids.ApprovalID{UUID: ids.NewV7()}

	for _, tc := range []struct {
		what   string
		staged workflow.StagedApprovalError
	}{
		{"a fresh proposal", workflow.StagedApprovalError{ApprovalID: id, Summary: summary}},
		{"one a human already answered", workflow.StagedApprovalError{
			ApprovalID: id, AlreadyApproved: true, Summary: summary,
		}},
	} {
		t.Run(tc.what, func(t *testing.T) {
			said := tc.staged.Error()
			if !strings.Contains(said, summary) {
				t.Errorf("the answer does not say what was staged, so an agent can only "+
					"report that something is pending:\n%s", said)
			}
			if !strings.Contains(said, id.String()) {
				t.Errorf("the answer does not name the approval:\n%s", said)
			}
		})
	}
}

// A producer that stages without describing what it staged is a real state —
// the automation engine's effects arrive with no card summary — and must not
// grow a placeholder sentence saying nothing.
func TestAStagedAnswerWithNoSummaryDoesNotInventOne(t *testing.T) {
	id := ids.ApprovalID{UUID: ids.NewV7()}
	bare := (&workflow.StagedApprovalError{ApprovalID: id}).Error()
	described := (&workflow.StagedApprovalError{ApprovalID: id, Summary: "Archive contact Ada"}).Error()

	// Asserted against the described rendering rather than against a joining
	// word: what must not happen is a placeholder standing where a description
	// belongs, and the only text that can tell those apart is the description.
	if len(bare) >= len(described) {
		t.Errorf("the undescribed answer is no shorter than the described one, so something "+
			"is standing in for the summary:\n%s", bare)
	}
	if !strings.Contains(bare, "staged as approval") || !strings.Contains(bare, id.String()) {
		t.Errorf("the answer stopped saying the call was staged and which approval it is:\n%s", bare)
	}
}

// A summary carries CALLER values — a quoted message body, a field list — and
// the answer lands in a transcript whose later prompts the same run reads.
// Unbounded, a caller chooses how much this server writes back at it.
func TestAStagedAnswerBoundsTheSummaryItRepeats(t *testing.T) {
	flood := strings.Repeat("é", 4000)
	said := (&workflow.StagedApprovalError{
		ApprovalID: ids.ApprovalID{UUID: ids.NewV7()}, Summary: flood,
	}).Error()

	if len(said) > workflow.MaxStagedSummary+400 {
		t.Errorf("a 4000-byte summary produced a %d-byte answer, so the caller chose how "+
			"much the server writes", len(said))
	}
	if !utf8.ValidString(said) {
		t.Error("the bound cut a multi-byte rune in half, so the answer is not valid UTF-8")
	}
	if !strings.Contains(said, "…") {
		t.Error("the answer was cut and does not say so")
	}
}
