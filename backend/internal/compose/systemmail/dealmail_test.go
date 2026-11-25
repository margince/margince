// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemmail

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension"
)

func TestAMailCursorOnlyReadsBackWhatTheReadGave(t *testing.T) {
	at := position{at: clocktest.Now(t).UTC().Truncate(time.Microsecond), id: ids.NewV7()}
	back, err := parseCursor(at.cursor())
	if err != nil || back == nil || !back.at.Equal(at.at) || back.id != at.id {
		t.Fatalf("a cursor read back as %+v, %v; want %+v", back, err, at)
	}
	for _, bad := range []string{"!!", "bm90LWEtY3Vyc29y"} {
		if _, err := parseCursor(bad); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("cursor %q: err = %v, want ErrInvalid", bad, err)
		}
	}
}
