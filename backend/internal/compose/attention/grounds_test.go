// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import "testing"

func TestATaskBodyGivesItsFirstParagraphAsTheReason(t *testing.T) {
	t.Parallel()
	body := "## Why\n\nAnna Weber wrote that the rollout moves to March.\nReview the close date.\n\nOlder notes follow."
	got := GroundsFromProse(body)
	if got == nil {
		t.Fatal("a body with prose gave no reason")
	}
	if want := "Anna Weber wrote that the rollout moves to March. Review the close date."; got.Text != want {
		t.Fatalf("got %q, want %q", got.Text, want)
	}
	got = GroundsFromProse("\r\n\r\n- Anna Weber wrote that the rollout moves to March.\r\n\r\nMore.")
	if want := "Anna Weber wrote that the rollout moves to March."; got == nil || got.Text != want {
		t.Fatalf("got %+v, want %q", got, want)
	}
}

func TestATaskWithoutProseHasNoReason(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"", "  \n\n \n", "#\n\n- "} {
		if got := GroundsFromProse(body); got != nil {
			t.Errorf("body %q gave reason %q, want none", body, got.Text)
		}
	}
}
