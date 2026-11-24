// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";
import { usePrefersReducedMotion } from "../design-system/motion";
import { RailLine } from "./agentrail-line";
import { type SpokenLine, spokenText } from "./ai-activity-speak";

/**
 * The one line under the orb. In one cell, the old reading fades out before the
 * new one fades in, so the slot never cuts and the two-line room holds still.
 *
 * A sentence is the same while its words are: the block re-renders on every
 * read. One outgoing layer at most, since the working state ticks faster than
 * the fade, and none under reduced motion.
 */
export function RailSaying({ line }: Readonly<{ line: SpokenLine }>) {
  const said = spokenText(line);
  const reduced = usePrefersReducedMotion();
  const [held, setHeld] = useState({ said, line });
  const [leaving, setLeaving] = useState<SpokenLine | null>(null);
  if (held.said !== said) {
    // Set during render: an effect would paint one frame of the new sentence
    // alone, the cut this removes.
    setLeaving(reduced ? null : held.line);
    setHeld({ said, line });
  } else if (reduced && leaving !== null) {
    // Reduced motion turned on mid-fade: the stylesheet cancels the animation,
    // so no `animationend` arrives to retire the old layer.
    setLeaving(null);
  }
  return (
    <span className="arswap">
      {/* Keyed on the words: a new element is what replays the fade. */}
      <span className="arline" key={said}>
        <RailLine line={line} />
      </span>
      {leaving !== null && (
        <span
          className="argone"
          key={spokenText(leaving)}
          // `inert` too: the leaving sentence can carry the record's link, and
          // a hidden control that takes Tab is a trap.
          aria-hidden="true"
          inert
          // The fade's end retires it, so the duration lives in the stylesheet.
          onAnimationEnd={() => setLeaving(null)}
        >
          <RailLine line={leaving} />
        </span>
      )}
    </span>
  );
}
