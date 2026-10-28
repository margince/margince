// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package signals

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The edit request reaches the store with the kind and the evidence next to
// the fields it already carried, so a correction is not dropped on the way.
func TestUpdateInputCarriesKindAndEvidence(t *testing.T) {
	kind := crmcontracts.UpdateSignalRequestKindRisk
	evidence := []crmcontracts.SignalEvidence{{Snippet: "the buyer asked to pause"}}
	version := int64(3)

	in := updateInput(crmcontracts.UpdateSignalRequest{Kind: &kind, Evidence: &evidence}, &version)

	if in.Kind == nil || *in.Kind != "risk" {
		t.Fatalf("kind = %v, want risk", in.Kind)
	}
	if in.Evidence == nil || len(*in.Evidence) != 1 || (*in.Evidence)[0].Snippet != "the buyer asked to pause" {
		t.Fatalf("evidence = %v, want the one claim sent", in.Evidence)
	}
	if in.IfVersion == nil || *in.IfVersion != version {
		t.Fatalf("if-version = %v, want %d", in.IfVersion, version)
	}
}

// A refused kind names the field and every kind an edit may set.
func TestInvalidSignalKindAnswersWithTheAllowedKinds(t *testing.T) {
	var refusal error = &InvalidSignalKindError{}
	var fault interface {
		FieldFault() (string, string, string)
	}
	if !errors.As(refusal, &fault) {
		t.Fatal("the refusal does not carry a field fault")
	}
	field, code, message := fault.FieldFault()
	if field != signalKindArg || code != "invalid_kind" {
		t.Fatalf("fault = %q %q, want %q invalid_kind", field, code, signalKindArg)
	}
	for _, kind := range correctableKinds {
		if !strings.Contains(message, kind) {
			t.Fatalf("message %q does not name %q", message, kind)
		}
	}
}
