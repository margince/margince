// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package usecases drives the six scenarios the product is sold on, over MCP,
// as an assistant meets them.
//
// The scenarios and their acceptance criteria are written down outside this
// repo, in the working folder's findings/ACCEPTANCE-CRITERIA.md. Each test
// below names the case and the criterion it pins, so a failure reads as
// "case 4, criterion 5" rather than as a Go function name nobody can map back
// to a promise:
//
//  1. Log it while it is fresh   — a meeting becomes records, and what was
//     promised in it becomes proposals
//  2. The business card          — one contact, never a second copy
//  3. The spreadsheet arrives    — an import that tells the truth twice
//  4. Use the moment             — who is nearby, and whose account it is
//  5. Before the meeting         — a briefing that names people and dates
//  6. Ask the company            — history answered from records, not prose
//
// # What is asserted, and from where
//
// Everything the ASSISTANT can see, over the transport it actually uses. A
// scenario calls tools through apptest.MCPClient and reads the answers as
// JSON. Where a criterion is about a human's half of the journey — approving
// something, undoing an import — that step is a REST call on the cookie
// client, because every /mcp request binds an agent principal and there is no
// cookie-authenticated tool call.
//
// The database is read only to establish what a fixture seeded, or to prove a
// write LANDED. It is never read to satisfy a criterion about what the caller
// was told: "the owner's name is in the row" and "the owner's name reached the
// client" are different claims, and only the second is the product.
//
// # What is NOT asserted
//
// The host's half of every scenario. The recording is fetched by the
// assistant, the card is read by the assistant's camera, the calendar is the
// assistant's. Those arrive here as text and fields, which is the same
// boundary the deck's own hosts draw — the server cannot tell a pasted
// transcript from an uploaded one, by design, and the contract says so.
//
// Whether a MODEL answers well is also out of scope. These tests pin what is
// deterministic: the payloads, the refusals, the staging, and the legibility
// fields a model needs in order to answer well at all. The model half runs
// separately, on a schedule, against a live assistant.
//
// # Isolation
//
// apptest.SetupApp* resets the shared package database per setup, so: ONE app
// per top-level scenario, and no t.Parallel in this package. Two scenarios
// sharing a database would see each other's writes, and case 3's whole point
// is that a second import changes nothing.
package usecases
