// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package assignments owns responsibility: who is accountable for a company, a
// deal or a project, and the administered vocabulary of roles they hold it
// under.
//
// Responsibility is deliberately NOT access. identity owns record_grant, which
// is what widens visibility; this module never writes it and never changes
// primary ownership. Naming a colleague the technical contact on a deal records
// a fact about the work and hands them nothing they could not already open.
//
// Authority comes from the parent record, not from a permission of this
// module's own: reading a record's assignments needs read access to that
// record, and writing one needs write access to it. That is why a caller who
// cannot see the parent gets 404 rather than an empty list.
//
// Tables owned: record_assignment, record_role.
//
// Imports shared + platform + the generated contract only; never a sibling
// module. The parent-visibility checks are ports bound at the composition root.
package assignments
