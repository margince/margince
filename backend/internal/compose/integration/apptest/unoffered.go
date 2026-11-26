// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import "github.com/margince/margince/backend/internal/modules/agents"

// The suites built on this harness call each verb by its own name over MCP.
// The role sweep must, to measure one verb at a time. The door that refuses a
// name tools/list leaves out is held by the agents package's own tests.
func init() { agents.ServeUnofferedNamesForTest() }
