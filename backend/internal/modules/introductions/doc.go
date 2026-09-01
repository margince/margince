// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package introductions owns the ask: one rep requesting that a colleague open
// a door to a contact, the colleague's bounded answer, and what actually
// happened afterwards.
//
// Three surfaces speak about introductions and only this one writes. The
// network graph says which routes EXIST and ranks them; the signals module
// PROPOSES a warm path off a fresh signal and stages nothing. Neither records
// that anybody was asked, which is why a rep could open a route somebody had
// already tried and been refused.
//
// The statuses are deliberately not collapsible. `name_drop_approved` — a
// colleague saying "use my name" — is not `accepted`, and `name_dropped` is
// not `introduced`: permission to mention somebody is a different event from a
// handshake, and a screen that reported one as the other would tell a rep a
// door had been opened that nobody opened. `replied` is reached only from
// captured activity, never from a checkbox, so the word means what it says.
//
// Tables owned: intro_request
package introductions
