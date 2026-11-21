// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import "github.com/margince/margince/backend/internal/compose/integration"

// The further legal entities these suites describe follow the at pack.
func init() { integration.RegisterEntityLawPack("at") }
