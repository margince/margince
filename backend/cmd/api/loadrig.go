// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build loadrig

package main

import (
	"os"

	"github.com/margince/margince/backend/internal/compose"
)

// A load-test build reads where its fake Gmail server listens before anything
// else runs; docs/how-to/measure-mail-import.md has the steps.
func init() {
	if base := os.Getenv("MARGINCE_LOADRIG_GOOGLE_BASE"); base != "" {
		compose.PointGoogleCallsAt(base)
	}
}
