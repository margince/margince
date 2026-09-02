// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The query keys the person record answers to.
//
// SEPARATE FROM THE SCREEN because another surface composes these addresses and
// the screen reads them, and the screen is lazy-loaded: importing it for one
// string would pull the whole person page into the worklist's chunk. One
// spelling either way — two would be a link that opens nothing.

/** Which meeting to brief, on arrival. */
export const BRIEF_PARAM = "prep";

/**
 * Asking the composer to open, and what it should be about.
 *
 * `?compose=reply` — the same intent vocabulary a moment action's prefill uses,
 * so a link and a rung ask for the draft the same way.
 *
 * WHAT IT DOES NOT SAY IS WHICH MESSAGE. The composer drafts to the PERSON,
 * choosing its transport from their own reachability rather than from a thread
 * a caller names, so an address cannot open it on one message. A parameter
 * naming an activity would promise a precision the composer does not have.
 */
export const COMPOSE_PARAM = "compose";
