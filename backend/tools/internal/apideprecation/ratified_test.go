// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apideprecation

import (
	"strings"
	"testing"
	"time"
)

func TestARatifiedRemovalNeedsNoNoticeAndEveryOtherRemovalStillDoes(t *testing.T) {
	at := time.Date(2026, 4, 30, 12, 0, 0, 0, time.UTC)
	before := contract(t, "      summary: Still supported\n")
	after := contract(t, "")
	ratified, err := ParseRatifiedRemovals(strings.NewReader("# comment\nGET /v1/old ratified-by-the-maintainers\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckChangesRatified(before, after, "", at, ratified); err != nil {
		t.Fatalf("a ratified removal was refused: %v", err)
	}
	other, err := ParseRatifiedRemovals(strings.NewReader("POST /v1/old another-operation\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckChangesRatified(before, after, "", at, other); err == nil {
		t.Fatal("a removal of an operation nobody ratified passed")
	}
}

func TestARatifiedRemovalStatesItsOperationAndReason(t *testing.T) {
	for _, text := range []string{"GET /v1/old\n", "GET\n", "GET /v1/old why\nget /v1/old again\n"} {
		if _, err := ParseRatifiedRemovals(strings.NewReader(text)); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}
