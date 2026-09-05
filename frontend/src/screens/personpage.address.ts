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
 * WHICH MESSAGE is the THREAD_PARAM below, and the two are separate keys
 * because they answer separately: an address may ask for the composer without
 * naming a conversation, and every caller that names one is also asking for it.
 */
export const COMPOSE_PARAM = "compose";

/**
 * Which conversation the composer should open on.
 *
 * The activity id of the message being answered. Without it the composer drafts
 * to the PERSON and picks its transport from their own reachability — which is
 * right for a rep who pressed Write on the record, and wrong for a worklist row
 * that is about ONE message: a contact reachable on two channels would have the
 * reply drafted into whichever they lead with.
 *
 * A thread this contact can no longer be answered on is not silently swapped
 * for another. The composer opens on the default and says the named
 * conversation is gone — see transportForActivity, which returns that as its
 * own answer rather than as an absent transport.
 */
export const THREAD_PARAM = "thread";
