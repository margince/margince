// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestExtensionOfNamesTheUnitFromTheInvocation(t *testing.T) {
	for name, tc := range map[string]struct {
		ctx  context.Context
		want string
	}{
		"no extension": {context.Background(), ""},
		"a unit":       {provenance.WithExtension(context.Background(), provenance.Extension{Unit: "books"}), "books"},
	} {
		if got := extensionOf(tc.ctx); got != tc.want {
			t.Errorf("%s: extensionOf = %q, want %q", name, got, tc.want)
		}
	}
}
