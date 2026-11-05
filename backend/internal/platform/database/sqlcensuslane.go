// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database

import "os"

// The integration lane is its own composition root: every pool a lane binary
// opens, through any harness, records into the one directory the lane names.
func init() {
	censusDir = os.Getenv(SQLCensusDirEnv)
}
