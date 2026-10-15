// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
)

func TestTagsSplitOnSemicolonsAndCollapseRepeats(t *testing.T) {
	if got := splitTags(" Partner; DACH;;partner "); !slices.Equal(got, []string{"Partner", "DACH"}) {
		t.Fatalf("got %v", got)
	}
	if got := splitTags("  Key   account "); !slices.Equal(got, []string{"Key account"}) {
		t.Fatalf("got %v, want inner spaces collapsed as the tag store does", got)
	}
	if got := splitTags(" ; "); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

func TestTagsAreNotComparedAgainstTheRecord(t *testing.T) {
	changed, err := changedFields([]byte(`{"display_name":"Acme"}`), map[string]string{csvTargetTags: "Partner"}, nil)
	if err != nil || len(changed) != 0 {
		t.Fatalf("changed = %v err %v, want none: tags are added beside the write, never diffed", changed, err)
	}
}
