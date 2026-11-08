// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai/aitest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// refBeside is aitest.RefBeside for a test that cannot go on without the ref.
func refBeside(t *testing.T, req model.Request, name string) string {
	t.Helper()
	ref, ok := aitest.RefBeside(req, name)
	if !ok {
		t.Fatalf("the request shows no ref beside %q:\n%v", name, req.Messages)
	}
	return ref
}
