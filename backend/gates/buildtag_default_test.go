// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

// builtWithIntegrationTag is false here; see buildtag_integration_test.go for
// why the package has to be able to answer this at all.
const builtWithIntegrationTag = false
