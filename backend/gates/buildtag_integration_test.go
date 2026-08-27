// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package gates

// builtWithIntegrationTag is how this package answers "which files did the
// compiler take?" — Go offers no way to read a binary's own build tags, and the
// parallel census has to ask, because thirteen gates here are `!integration`
// and a census that judged files the build excluded would report tests Go never
// runs. Two four-line files are the whole mechanism.
const builtWithIntegrationTag = true
