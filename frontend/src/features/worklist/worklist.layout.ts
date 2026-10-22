// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Where a block stands on a PHONE, declared by the block.
//
// The page reads header → readings → the day → what follows the day, and on a
// narrow screen the readings have to move: four stat cards stack into a 338px
// block, and above the queue they put the first row's verb below the fold. The
// stylesheet moves them with `order`, which needs to know what they should land
// in FRONT of — and a selector listing today's trailing panels would go stale
// the day the page grows another.
//
// So the fact travels with the block instead. A panel that belongs below the
// day says so here; one that forgets paints above the readings, which is a
// wrong a reader can see rather than a silent one.
//
// Paint order only. It moves nothing in the document, so a screen reader still
// meets the day's figures before the sections they frame.

/** The class on every block that stands BELOW the reader's own day. */
export const AFTER_THE_DAY = "worklist-after-day";
