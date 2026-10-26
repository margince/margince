// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CSSProperties, ReactNode } from "react";
import { type PeekContent, usePeek } from "./peek";
import { type ReadingTone, readingColours } from "./readingtone";
import "./standingdims.css";

type DimVars = CSSProperties & Record<`--${string}`, string>;

/**
 * One dimension of a record's standing. `value` is the reading in words or a
 * count ("1 overdue", "Not rated"); `sub` is what it rests on, one line under
 * it. `tone` colours the dot and the value; `neutral` is a dimension nobody
 * rated.
 */
export type StandingDim = Readonly<{
  key: string;
  label: ReactNode;
  value: ReactNode;
  sub?: ReactNode;
  tone: ReadingTone;
  // Who or which the reading counts. Absent, the row is plain words; present,
  // the reading opens a card of them on hover, focus or a press.
  peek?: StandingPeek;
}>;

/** The card behind a reading. `on` is the line that holds the count, so that
 *  line wears the mark: "3 contacts here are in touch" is the sub-line under
 *  the word "Strong". */
export type StandingPeek = PeekContent &
  Readonly<{ on?: "value" | "sub" | "both" }>;

/**
 * The three readings beside the hero of How it stands: a dot, the
 * dimension's name, and its value with the line it rests on.
 */
export function StandingDims({
  dims,
  label,
}: Readonly<{
  dims: readonly StandingDim[];
  // What the rows are readings of, for a screen reader.
  label?: string;
}>) {
  return (
    <dl className="standing-dims" aria-label={label}>
      {dims.map((dim) => (
        <Dim key={dim.key} dim={dim} />
      ))}
    </dl>
  );
}

function Dim({ dim }: Readonly<{ dim: StandingDim }>) {
  const colours = readingColours(dim.tone);
  const vars: DimVars = {
    "--standing-dim-mark": colours.mark,
    "--standing-dim-ink": colours.ink,
  };
  const card = dim.peek && dim.peek.items.length > 0 ? dim.peek : undefined;
  const {
    ref: peekRef,
    props: peekProps,
    panel: peekPanel,
    open: peekOpen,
  } = usePeek<HTMLButtonElement>({ content: card, pressOpens: true });
  const reading = (
    <>
      <span className="standing-dim-value">{dim.value}</span>
      {dim.sub ? (
        <span
          className="standing-dim-sub"
          title={typeof dim.sub === "string" && !card ? dim.sub : undefined}
        >
          {dim.sub}
        </span>
      ) : null}
    </>
  );
  return (
    <div className="standing-dim" data-tone={dim.tone} style={vars}>
      <dt className="standing-dim-label">
        <span className="standing-dim-dot" aria-hidden="true" />
        {dim.label}
      </dt>
      <dd className="standing-dim-reading">
        {card ? (
          <>
            <button
              ref={peekRef}
              type="button"
              className="standing-dim-peek"
              data-peek-on={card.on ?? "value"}
              aria-haspopup="true"
              aria-expanded={peekOpen}
              {...peekProps}
            >
              {reading}
            </button>
            {peekPanel}
          </>
        ) : (
          reading
        )}
      </dd>
    </div>
  );
}
