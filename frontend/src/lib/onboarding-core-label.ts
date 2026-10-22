// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MarginceCoreState } from "../design-system/margince-core";
import type { MessageKey } from "../i18n/en";

/**
 * The Core's state, named in words — one table, read by every surface that
 * draws the orb.
 *
 * WHY IT EXISTS AT ALL. The Core is `aria-hidden` and its state is motion first
 * (WDS-CORE-4), so a surface that draws it owes the reader the same fact in
 * text. That obligation is per surface; the WORDS are not, and two screens
 * showing `ingest` must not describe it differently — an orb that reads as
 * "taking it in" on one screen and something else on the next is two states as
 * far as anybody reading is concerned.
 *
 * WHY A TABLE AND NOT A FUNCTION. `Record<MarginceCoreState, …>` is total in
 * both directions: a state added to the closed list with no words fails to
 * compile, and words for a state that does not exist fail too. A `switch` with a
 * default would quietly cover the first case with a placeholder.
 *
 * NOT the read's phase line. What a read is doing for the reader ("Fetching
 * pages") is the theatre's own sentence; this says what the ORB is doing, and
 * one fact in two places is two places to keep agreeing.
 */
export const CORE_LABELS: Readonly<Record<MarginceCoreState, MessageKey>> = {
  idle: "ob.core.idle",
  ingest: "ob.core.ingest",
  working: "ob.core.working",
  warning: "ob.core.warning",
  error: "ob.core.error",
};
