// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect, useRef, useState } from "react";
import { formatNumber } from "../format/format";
import type { Locale } from "../i18n";
import { usePrefersReducedMotion } from "./motion";
import "./countup.css";

/**
 * A number that arrives rather than appears.
 *
 * FOR A COUNT THAT IS STILL BEING EARNED, and only that. A figure the server
 * settled long ago has nothing to count towards, and animating it is decoration
 * pretending to be work. Here the count is the work: pages read while somebody
 * waits, and the run is what tells them the wait is going somewhere.
 *
 * IT COUNTS FROM WHERE IT WAS, not from zero. The value climbs as a read
 * progresses, and restarting at zero on every poll would show a number falling
 * to nothing several times a minute, which reads as a reset rather than as
 * progress. The first arrival is the one exception: nothing was on screen
 * before it, so it runs up from zero.
 *
 * UNDER REDUCED MOTION IT IS THE NUMBER. Not a stalled zero, not a fade: the
 * end state, immediately, which is the rule the whole motion module holds.
 */
export function CountUp({
  value,
  locale,
  className,
}: Readonly<{
  value: number;
  /**
   * For grouping, so a four-figure count reads as one in the reader's own way.
   *
   * The product's own locale rather than an Intl tag, and formatted through the
   * one formatter: a component that reached for `toLocaleString` itself would be
   * a second answer to which locale a number is written in, and the one that
   * disagrees is always the one nobody is looking at.
   */
  locale: Locale;
  className?: string;
}>) {
  const reduced = usePrefersReducedMotion();
  const [shown, setShown] = useState(reduced ? value : 0);
  // What the last run ENDED on, which is where the next one starts. A ref
  // rather than state: it is read inside the frame loop and changing it must
  // not itself schedule a render.
  const from = useRef(reduced ? value : 0);

  useEffect(() => {
    if (reduced) {
      from.current = value;
      setShown(value);
      return;
    }
    const start = from.current;
    if (start === value) {
      return;
    }
    const t0 = performance.now();
    let frame = 0;
    const tick = (now: number) => {
      const k = Math.min(1, (now - t0) / COUNT_MS);
      // Cubic ease-out: fast enough to read as arriving, slow enough at the end
      // that the final figure settles rather than snapping.
      const eased = 1 - (1 - k) ** 3;
      setShown(Math.round(start + (value - start) * eased));
      if (k < 1) {
        frame = requestAnimationFrame(tick);
        return;
      }
      from.current = value;
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [value, reduced]);

  return (
    <span className={className ? `countup ${className}` : "countup"}>
      {formatNumber(shown, locale)}
    </span>
  );
}

/** Long enough to read as a climb, short enough that a poll does not overtake it. */
const COUNT_MS = 1100;
