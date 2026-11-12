// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agents

// ServeUnofferedNamesForTest makes tools/call serve every registered name, as
// the role sweep needs to measure each verb on its own.
func ServeUnofferedNamesForTest() { servesUnoffered.Store(true) }
