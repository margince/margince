// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package e2e is the parent of the end-to-end scenario suites: whole user
// journeys driven from OUTSIDE the product, one subdirectory per area.
//
// It holds no tests of its own. The suites live one level down —
// e2e/usecases/ is the first — so that an area's fixtures, helpers and
// scenarios sit together and a second area does not inherit the first's.
//
// # What belongs here, and what does not
//
// The integration lane already has eleven topic packages beside this one
// (agentaccess, capture, channels, org360, …). They are the right home for a
// SUBSYSTEM: the connector's discovery chain, the capture pipeline, the
// custom-field resolver. Each asks whether one part of the product is correct.
//
// A suite belongs HERE instead when it asks whether a JOURNEY works — several
// subsystems in sequence, in the order and by the route a person actually
// meets them, with the assertions written from what the user was promised
// rather than from what a package exports. The distinction is the direction
// the test is written from, not how many packages it touches.
//
// Two consequences follow, and both are why a separate parent is worth having:
//
//   - A scenario asserts on what a CALLER can see. If a fact is only reachable
//     by importing an internal package, that is a finding about the product
//     rather than a reason to reach for the import. Four defects merged in
//     August 2026 were of exactly this shape — data present, correct, and
//     illegible to the client holding it.
//
//   - A scenario is allowed to be slow and few. A subsystem package earns its
//     place by covering a matrix; a journey earns its place by being the
//     journey. Adding the twelfth variation of case 4 is usually the wrong
//     instinct — the variation belongs beside the subsystem that varies.
//
// # Adding an area
//
// Create e2e/<area>/ with its own doc.go saying which journeys it covers and
// what shape they are driven from. Nothing needs registering: the integration
// sharder discovers packages by walking the tree for the `integration` build
// tag, so a new directory joins the lane by existing.
//
// Keep each area's fixtures inside that area. A helper shared by two areas
// moves up to apptest, which is where the harness lives; it does not move
// sideways into a sibling, and it does not accumulate here.
package e2e
