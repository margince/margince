// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The lead record's own address dials.
//
// Its own file for the reason `personpage.address.ts` is one: a caller that
// builds a link to this screen must not import the screen to learn what the
// address is called, and a parameter spelled twice is a link that works from
// one surface and not the other.

/** The dial naming the verb a caller sent the reader here to perform. */
export const ACTION_PARAM = "action";

/**
 * Arrive with the call composer ready.
 *
 * A call attempt is the ordinary answer to a lead nobody has replied to yet,
 * and it is the one activity kind a rep logs from somewhere else rather than
 * from the record they were already reading.
 */
export const CALL_ACTION = "call";
