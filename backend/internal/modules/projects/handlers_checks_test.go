// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
)

func TestAProjectWriteLeavesEveryCheckButACustomPicklistToTheSharedNet(t *testing.T) {
	t.Parallel()
	faulttest.AssertCheckViolationsReachTheSharedNet(t, projectObject, writeStoreErr)
}
