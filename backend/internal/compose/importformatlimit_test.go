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
