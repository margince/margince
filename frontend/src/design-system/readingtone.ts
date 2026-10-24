// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { STAT_CARD_TONES } from "./statcard";
import type { StrengthBand } from "./strengthmeter";

/**
 * How a reading on a record stands. It covers the verdict of How it stands, one
 * dimension row, one figure, one arc of a ring, and the current rung of a ladder.
 * The state words the rest of the catalog already uses, plus `neutral`. That
 * is for a reading nobody has judged ("Not rated", "Not assessed"), and it is
 * never drawn in a state colour.
 */
export const READING_TONES = [
  "success",
  "warning",
  "danger",
  "neutral",
] as const;

export type ReadingTone = (typeof READING_TONES)[number];

type ToneColours = Readonly<{ mark: string; ink: string; wash: string }>;

// One table, so the dot, the arc, the word and the card's wash of one tone
// cannot drift apart across the five record pages.
const COLOURS: Readonly<Record<ReadingTone, ToneColours>> = {
  success: {
    mark: "var(--success)",
    ink: "var(--successText)",
    wash: "var(--successBg)",
  },
  warning: {
    mark: "var(--warning)",
    ink: "var(--warningText)",
    wash: "var(--warningBg)",
  },
  danger: {
    mark: "var(--danger)",
    ink: "var(--dangerText)",
    wash: "var(--dangerBg)",
  },
  neutral: {
    mark: "var(--borderStrong)",
    ink: "var(--textSecondary)",
    wash: "transparent",
  },
};

/** The colours of one tone: `mark` fills a dot or an arc, `ink` letters a
 *  word, `wash` tints a card's ground. */
export function readingColours(tone: ReadingTone): ToneColours {
  return COLOURS[tone];
}

/** The same tone in `StatCard`'s vocabulary, where an unjudged figure takes
 *  no tone at all. */
export function statTone(
  tone: ReadingTone,
): (typeof STAT_CARD_TONES)[number] | undefined {
  return tone === "neutral" ? undefined : tone;
}

/** The health ratings as the API spells them (`HealthDimension.rating`), worst
 *  first: a record's verdict is the lowest-ranked rating among its rated
 *  dimensions. A dimension that was not rated has none. */
export const HEALTH_RATINGS = ["at_risk", "good", "strong"] as const;

export type HealthRating = (typeof HEALTH_RATINGS)[number];

/** What colour one health arc or dimension takes. Unrated is neutral, never
 *  the all-clear. */
export function ratingTone(rating: HealthRating | undefined): ReadingTone {
  switch (rating) {
    case "strong":
      return "success";
    case "good":
      return "warning";
    case "at_risk":
      return "danger";
    default:
      return "neutral";
  }
}

/** What colour the strength ring's fill takes. */
export function bandTone(band: StrengthBand): ReadingTone {
  switch (band) {
    case "strong":
      return "success";
    case "moderate":
      return "warning";
    case "weak":
      return "danger";
    default:
      return "neutral";
  }
}

/** The same tone as a `Badge` tone, where unjudged is the plain badge. */
export function badgeTone(
  tone: ReadingTone,
): "success" | "warning" | "danger" | "default" {
  return tone === "neutral" ? "default" : tone;
}
