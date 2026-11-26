// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database

import (
	"os"
	"sync/atomic"
)

// The integration lane is its own composition root: every pool a lane binary
// opens, through any harness, records into the one directory the lane names.
func init() {
	censusDir = os.Getenv(SQLCensusDirEnv)
}

var largestSQLStatement atomic.Int64

func observeStatementSize(sql string) {
	size := int64(len(sql))
	for previous := largestSQLStatement.Load(); size > previous; previous = largestSQLStatement.Load() {
		if largestSQLStatement.CompareAndSwap(previous, size) {
			return
		}
	}
}

// LargestSQLStatementBytes lets an integration benchmark bound statement size
// without file capture changing the latency it measures.
func LargestSQLStatementBytes() int64 { return largestSQLStatement.Load() }

// ResetLargestSQLStatementBytes excludes setup SQL from the measured read path.
func ResetLargestSQLStatementBytes() { largestSQLStatement.Store(0) }
