// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package initiatives owns the sales initiative: one commercial effort with an
// objective and a deadline. Rules or picks say which leads and deals belong,
// and a baseline of deals is frozen at the launch. Handlers→Store. The rules are
// predicate trees in the list vocabulary, evaluated through the matcher the
// composition root injects, so one filter engine answers lists and initiatives.
//
// Tables owned: docs/reference/entity-model/initiatives.md.
package initiatives
