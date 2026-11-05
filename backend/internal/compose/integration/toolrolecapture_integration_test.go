// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// captureFixtures fill the capture and connector calls: the user's lists, the
// workspace's domain decisions and the settings of a connected mailbox.
//
//craft:ignore naked-any an argument takes whichever JSON type its verb's schema names
var captureFixtures = map[string]func(p *recordPool) map[string]any{}
