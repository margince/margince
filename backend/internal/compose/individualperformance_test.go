// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// individualPerformanceShown is a request whose door bound the switch on, which is what a
// test of the lens itself asks about.
func individualPerformanceShown() context.Context {
	return auth.WithIndividualPerformance(context.Background(), true)
}
