// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package briefevidence attaches the canonical email row to the evidence a
// response cites, so a cited message can be opened as that message.
//
// Every writer in this tree that grounds prose in records — the company brief, the
// dossier, the growth fit, the person brief, the meeting brief, the deal status
// card, both draft services and the account scan — emits the same citation
// shape: a record kind and a record id. That is enough to name a message and
// not enough to open one, so eight surfaces drew a citation a reader could
// press and nothing happened behind it.
//
// The missing half is the reader-scoped email row, and it is missing for a
// reason: it cannot be written where the evidence is. A brief is cached, a deal
// card is saved, an account scan's findings are persisted — and an email
// summary is assembled for ONE reader out of that reader's own audience and
// content grants. A stored summary would serve the first reader's access to the
// second, which is why this package attaches AFTER any save and why nothing it
// produces is ever written back.
//
// One Attach per response, one read behind it. The producers differ in shape —
// sentences, suggestions, draft reasons, a deal's next move — so each has a
// collector that turns its own slices into Targets, and the batching rule lives
// here rather than in each of the nine wire types.
package briefevidence
