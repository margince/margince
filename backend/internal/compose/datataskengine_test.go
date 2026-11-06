// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestCorrectionFailureNoticeNamesTheTaskAndItsCause(t *testing.T) {
	class := "statement_stopped"
	subject, body := dataTaskFailureWords(datatask.Progress{
		TaskID: "core/100_example", Title: "Mapping company industries onto the fixed list",
		Step: "remap", ConsecutiveFailures: 5, LastErrorClass: &class,
	})
	if subject != "Background correction stopped: Mapping company industries onto the fixed list" {
		t.Fatalf("subject=%q", subject)
	}
	for _, want := range []string{"step remap", "5 failed attempts", "Last failure: the database stopped a statement", "System health"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %q lacks %q", body, want)
		}
	}
}

func TestCorrectionFailureNoticeSaysWhenTheCauseIsUnknown(t *testing.T) {
	subject, body := dataTaskFailureWords(datatask.Progress{TaskID: "core/100_example", Step: "first"})
	if subject != "Background correction stopped: core/100_example" {
		t.Fatalf("an untitled task is named by its id, got %q", subject)
	}
	if !strings.Contains(body, "could not be classified") || !strings.Contains(body, "repeated failures") {
		t.Fatalf("body=%q", body)
	}
}
