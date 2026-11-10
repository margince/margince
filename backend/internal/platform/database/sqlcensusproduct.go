// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package database

// Statement-size counting belongs to the integration benchmark build.
func observeStatementSize(string) {}
