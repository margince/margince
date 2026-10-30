// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Sparkles } from "lucide-react";
import { PendingBody } from "./atoms";
import "./aipending.css";

/**
 * AiPending is the wait for something Margince is reading or writing: the
 * indigo tile breathing where the agent's mark will stand, rows of drifting
 * specks where the answer's lines will go, and the sentence saying what is
 * being read. When the answer lands, `AiReveal` carries its words in. It composes PendingBody rather than restating it — the announcement
 * and the pulse each have one home — and adds the one thing a plain
 * placeholder cannot say: that a MACHINE is at work, and on what.
 *
 * The tile is the claim of authorship (the same tile a finished 360 opens
 * with). The motion is opacity, transform and background position only, and
 * under reduced motion the tile and the specks rest, while the lines keep the
 * placeholder's own shape — the shape is what says "this is coming".
 *
 * `label` is REQUIRED for PendingBody's reason: it is the only line a reader
 * who cannot see the tile gets, and only the caller knows what is being read.
 * It is shown as well as spoken, because this wait is the long kind — a model
 * call is upwards of twenty seconds cold, and a mute grey block that long
 * reads as broken.
 */
export function AiPending({
  label,
  lines = 3,
}: Readonly<{
  label: string;
  // How many rows of content will stand here once the read answers — a
  // height reservation, so the card does not jump when the findings land.
  lines?: number;
}>) {
  return (
    <div className="aipending">
      <span className="aipending-tile" aria-hidden="true">
        <Sparkles />
      </span>
      <div className="aipending-body">
        <PendingBody label={label} lines={lines} visible />
      </div>
    </div>
  );
}
