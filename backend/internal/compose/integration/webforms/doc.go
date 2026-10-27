// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package webforms holds the website forms integration suites: an admin keeping
// forms, and an anonymous sender posting one into a lead with its note.
//
// It is split out of internal/compose/integration to give the lane another slot.
// One package is one slot, and the parent runs close to its budget. Its suites
// ride the parent's exported fixtures and integration/apptest's.
package webforms
