// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package contactfixture supplies identities without importing either HTTP or database harnesses.
package contactfixture

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// Profile gives each manual-create fixture a distinct LinkedIn identity.
func Profile() string {
	return "https://www.linkedin.com/in/" + ids.NewV7().String()
}
