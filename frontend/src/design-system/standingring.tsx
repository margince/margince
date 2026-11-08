// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { CSSProperties, ReactNode } from "react";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { type PeekContent, usePeek } from "./peek";
import {
  bandTone,
  type HealthRating,
  type ReadingTone,
  ratingTone,
  readingColours,
} from "./readingtone";
import { fillDash, RING, segmentDashes } from "./ringarc";
import type { StrengthBand } from "./strengthmeter";
import { type Tooltip, useTooltip } from "./tooltip";
import "./standingring.css";

const ARC_GAP_DEGREES = 4;

type RingVars = CSSProperties & Record<`--${string}`, string>;

/** One health dimension the ring draws an arc for, in the caller's order. */
export type HealthArc = Readonly<{
  key: string;
  label: string;
  rating?: HealthRating;
}>;

/**
 * The health reading as one ring: an arc per dimension, coloured by its
 * rating and left as bare track when it was not rated. The centre carries the
 * overall word and how many of the arcs were rated.
 */
export function HealthRing({
  arcs,
  word,
  tone,
}: Readonly<{
  arcs: readonly HealthArc[];
  // The overall reading in the page's own words ("Watch", "Not rated"). The
  // caller folds the ratings into it, so the ring and the verdict share one
  // rule. With nothing rated the word is only the verdict again, so the
  // centre shows the count alone.
  word: string;
  tone: ReadingTone;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const rated = arcs.filter((arc) => arc.rating !== undefined).length;
  const ratedLine = t("standing.ring.rated", {
    rated: formatNumber(rated, locale),
    total: formatNumber(arcs.length, locale),
  });
  const segments = segmentDashes(arcs.length, ARC_GAP_DEGREES);
  return (
    <RingFrame label={`${word}. ${ratedLine}`} tone={tone}>
      <svg viewBox={`0 0 ${RING.size} ${RING.size}`} aria-hidden="true">
        <Track />
        {arcs.map((arc, index) => {
          const segment = segments[index];
          return arc.rating === undefined || !segment ? null : (
            <circle
              key={arc.key}
              className="standing-ring-segment"
              {...CIRCLE}
              style={{ stroke: readingColours(ratingTone(arc.rating)).mark }}
              strokeDasharray={segment.dash}
              strokeDashoffset={segment.offset}
            />
          );
        })}
      </svg>
      <span className="standing-ring-centre">
        {rated > 0 && <span className="standing-ring-word">{word}</span>}
        <span className="standing-ring-caption">{ratedLine}</span>
      </span>
    </RingFrame>
  );
}

/**
 * Relationship strength as one ring: filled to the score out of 100 in the
 * colour of its band. A record with no interaction at all draws the bare
 * track and says so, never a zero.
 */
export function StrengthRing({
  score,
  band,
  bandLabel,
  detail,
  explain,
}: Readonly<{
  score: number;
  band: StrengthBand;
  // The band in the page's words ("strong").
  bandLabel: string;
  // The one fact the band rests on, under the band word. The bare score is
  // never drawn: a lone "9" asks the reader what it is out of.
  detail: string;
  // What the number is made of, shown on hover and focus.
  explain: string;
}>) {
  const t = useT();
  const tooltip = useTooltip<HTMLDivElement>(explain);
  const { locale } = useLocale();
  const tone = bandTone(band);
  if (band === "none") {
    const empty = t("standing.ring.noInteractions");
    return (
      <RingFrame label={empty} tone="neutral">
        <svg viewBox={`0 0 ${RING.size} ${RING.size}`} aria-hidden="true">
          <Track />
        </svg>
        <span className="standing-ring-centre">
          <span className="standing-ring-caption">{empty}</span>
        </span>
      </RingFrame>
    );
  }
  return (
    <RingFrame
      label={t("standing.ring.strength", {
        score: formatNumber(score, locale),
        band: bandLabel,
      })}
      tone={tone}
      tooltip={tooltip}
    >
      <svg viewBox={`0 0 ${RING.size} ${RING.size}`} aria-hidden="true">
        <Track />
        <circle
          className="standing-ring-fill"
          {...CIRCLE}
          style={{ stroke: readingColours(tone).mark }}
          strokeDasharray={fillDash(score / 100)}
        />
      </svg>
      <span className="standing-ring-centre">
        <span className="standing-ring-word">{bandLabel}</span>
        <span className="standing-ring-caption">{detail}</span>
      </span>
    </RingFrame>
  );
}

/**
 * A score out of 100 as one ring: filled to the score, the number in its
 * centre over the word for it. Only for a scale the reader already knows as
 * points, such as a lead's score; a blended index uses StrengthRing. The word
 * opens the points that make the score up (`peek`).
 */
export function ScoreRing({
  score,
  tone,
  label,
  caption,
  peek,
}: Readonly<{
  score: number;
  tone: ReadingTone;
  // The whole reading for a screen reader ("Score 82 of 100").
  label: string;
  // The word under the number ("Score").
  caption: string;
  peek?: PeekContent;
}>) {
  const { locale } = useLocale();
  const card = peek && peek.items.length > 0 ? peek : undefined;
  const {
    ref: peekRef,
    props: peekProps,
    panel: peekPanel,
    open: peekOpen,
  } = usePeek<HTMLButtonElement>({
    content: card,
    pressOpens: true,
  });
  return (
    <RingFrame label={label} tone={tone} interactive={card !== undefined}>
      <svg viewBox={`0 0 ${RING.size} ${RING.size}`} aria-hidden="true">
        <Track />
        {score > 0 && (
          <circle
            className="standing-ring-fill"
            {...CIRCLE}
            style={{ stroke: readingColours(tone).mark }}
            strokeDasharray={fillDash(Math.min(score, 100) / 100)}
          />
        )}
      </svg>
      <span className="standing-ring-centre">
        <span className="standing-ring-score" aria-hidden={card !== undefined}>
          {formatNumber(score, locale)}
        </span>
        {card ? (
          <button
            ref={peekRef}
            type="button"
            className="standing-ring-caption standing-ring-peek"
            aria-label={label}
            aria-haspopup="true"
            aria-expanded={peekOpen}
            {...peekProps}
          >
            {caption}
          </button>
        ) : (
          <span className="standing-ring-caption">{caption}</span>
        )}
      </span>
      {peekPanel}
    </RingFrame>
  );
}

const CIRCLE = {
  cx: RING.size / 2,
  cy: RING.size / 2,
  r: RING.radius,
  fill: "none",
  strokeWidth: RING.stroke,
} as const;

function Track() {
  return <circle className="standing-ring-track" {...CIRCLE} />;
}

// A ring holding a control is a group, since an image hides its children.
function RingFrame({
  label,
  tone,
  tooltip,
  interactive = false,
  children,
}: Readonly<{
  label: string;
  tone: ReadingTone;
  tooltip?: Tooltip<HTMLDivElement>;
  interactive?: boolean;
  children: ReactNode;
}>) {
  const vars: RingVars = { "--standing-ring-ink": readingColours(tone).ink };
  const named = { role: interactive ? "group" : "img", "aria-label": label };
  return (
    <div
      className="standing-ring"
      {...named}
      style={vars}
      ref={tooltip?.ref}
      tabIndex={tooltip ? 0 : undefined} // NOSONAR: the tab stop is how a keyboard reaches the explanation (WCAG 1.4.13)
      {...tooltip?.trigger}
    >
      {children}
      {tooltip?.tip}
    </div>
  );
}
