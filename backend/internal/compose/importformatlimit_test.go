// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"
)

func TestTheFormatIsRefusedWhereNoUploadCeilingWasConfigured(t *testing.T) {
	if _, err := importsOverDB(nil).DescribeFormat(context.Background()); !errors.Is(err, errUploadLimitUnset) {
		t.Fatalf("err = %v, want errUploadLimitUnset rather than a format that states a 0 MB limit", err)
	}
}

func TestTheFormatStatesTheCeilingInTheMegabytesTheUploadEnforces(t *testing.T) {
	for _, c := range []struct {
		limit int64
		want  int
	}{{10_000_000, 10}, {9_999_999, 9}, {1_000_000, 1}} {
		got, err := importHandlers{uploadLimit: c.limit}.uploadLimitMB()
		if err != nil || got != c.want {
			t.Errorf("a %d-byte ceiling states %d MB (%v), want %d: the refusal past it says MB of a million bytes", c.limit, got, err, c.want)
		}
	}
}
