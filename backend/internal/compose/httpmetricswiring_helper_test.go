// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "github.com/margince/margince/backend/internal/platform/httpserver"

// newHTTPMetricsForTest keeps the constructor call in one place, so a change to
// its signature does not scatter across the wiring tests.
func newHTTPMetricsForTest() *httpserver.HTTPMetrics { return httpserver.NewHTTPMetrics() }
