// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftreply_test

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftreply"
)

// A message a reader can send survives, whatever the model wrapped it in.
func TestASendableReplyIsRead(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"bare object": `{"subject":"Intro to Philipp?","body":"Hi Sofia, could you introduce me to Philipp Königs?"}`,
		"fenced":      "```json\n{\"subject\":\"Intro to Philipp?\",\"body\":\"Hi Sofia, could you introduce me to Philipp Königs?\"}\n```",
		"padded":      `{"subject":"  Intro to Philipp?  ","body":"  Hi Sofia, could you introduce me to Philipp Königs?  "}`,
	} {
		subject, body, err := draftreply.Parse(raw, "Sofia", "Philipp Königs")
		if err != nil {
			t.Fatalf("%s: refused a sendable reply: %v", name, err)
		}
		if subject != "Intro to Philipp?" {
			t.Errorf("%s: subject came back %q", name, subject)
		}
		if !strings.HasPrefix(body, "Hi Sofia,") {
			t.Errorf("%s: body came back %q", name, body)
		}
	}
}

// Each refusal exists because the reader would otherwise be handed something
// worse than the template: nothing to send, or a message they must rewrite.
func TestAReplyAReaderCannotSendIsRefused(t *testing.T) {
	t.Parallel()
	for name, raw := range map[string]string{
		"not the shape":  `not json at all`,
		"no subject":     `{"subject":"","body":"Hi Sofia, could you introduce me to Philipp Königs?"}`,
		"no body":        `{"subject":"Intro?","body":"   "}`,
		"names nobody":   `{"subject":"Intro?","body":"Could you introduce me to them?"}`,
		"names only one": `{"subject":"Intro?","body":"Hi Sofia, could you introduce me to someone there?"}`,
	} {
		if _, _, err := draftreply.Parse(raw, "Sofia", "Philipp Königs"); err == nil {
			t.Errorf("%s: accepted a reply the reader cannot send", name)
		}
	}
}

// A caller may pass a name it does not have — an absent fact asks nothing of
// the model, and refusing on it would fail every reply for a missing field.
func TestAnEmptyNameAsksNothing(t *testing.T) {
	t.Parallel()
	if _, _, err := draftreply.Parse(
		`{"subject":"Intro?","body":"Hi Sofia, could you introduce me?"}`, "Sofia", "",
	); err != nil {
		t.Fatalf("an absent name refused the reply: %v", err)
	}
}

// The refusal says which name is missing, because a reliability drop that does
// not name its cause is a number rather than a diagnosis.
func TestTheRefusalNamesWhoIsMissing(t *testing.T) {
	t.Parallel()
	_, _, err := draftreply.Parse(
		`{"subject":"Intro?","body":"Hi Sofia, could you introduce me?"}`, "Sofia", "Philipp Königs")
	if err == nil {
		t.Fatal("accepted a draft naming only one of two people")
	}
	if !strings.Contains(err.Error(), "Philipp Königs") {
		t.Errorf("the refusal does not name who is missing: %v", err)
	}
}
