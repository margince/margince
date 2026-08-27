// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// AnyMap is the shape a scenario builds an ad-hoc JSON request body in, and
// reads a response into when it asserts on one or two fields rather than a
// typed struct. An alias, not a new type, so it interchanges freely with the
// map literals the suites already write.
//
// Here rather than in apptest, whose subject is the booted application: a JSON
// literal has nothing to do with one, and fourteen files were importing that
// package for this alone — a dependency that said the suite needed a running
// server when all it needed was a map.
type AnyMap = map[string]any
