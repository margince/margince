// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Button } from "./atoms";
import { useTooltip } from "./tooltip";
import "./iconaction.css";

/**
 * A verb whose glyph is its whole label: square, named, and named again on
 * hover.
 *
 * `Button iconOnly` already draws the square and already documents that the
 * caller owes it an accessible name. What it cannot do is give a SIGHTED reader
 * that name — and a row of unlabelled glyphs is a row of guesses, which is the
 * reason icon-only buttons get talked out of. This pairs the two halves so they
 * cannot come apart: one `label`, spoken through `aria-label` and shown through
 * `useTooltip`, so the name a screen reader hears and the name a pointer reveals
 * are the same string and there is no second prop for them to disagree in.
 *
 * WHEN to reach for it, and it is not "whenever there is an icon": only for a
 * verb a reader already knows from the glyph — mail, phone, calendar, pencil,
 * link, the overflow ellipsis. A verb whose consequence a reader must read
 * before pressing (convert, merge, archive, disqualify) keeps its words, and a
 * verb that is rare enough to need explaining belongs in an overflow menu where
 * it can have a whole line. Squaring those saves a few pixels and costs the
 * reader the one thing they needed.
 *
 * The handlers ride a wrapping span rather than the button: focus and pointer
 * events reach it from the control inside, and the alternative is merging a
 * caller's ref and pointer handlers into `Button`'s own prop contract, which is
 * guarded on purpose against exactly that kind of arrival.
 */
export function IconAction({
  label,
  icon,
  variant,
  small,
  reason,
  reasonId,
  pending,
  pressed,
  onClick,
  testId,
}: Readonly<{
  /** The verb, translated. Spoken as the name and shown as the tip. */
  label: string;
  /** The glyph, `aria-hidden` — the label is what names this control. */
  icon: ReactNode;
  variant?: "primary" | "ghost" | "danger";
  small?: boolean;
  /** Why this verb is unavailable; refuses the press, as on `Button`. */
  reason?: string;
  /**
   * The id of a sentence ALREADY on the page saying why — for a surface where
   * one fact refuses several verbs, as `Button.reasonId` is for.
   */
  reasonId?: string;
  pending?: boolean;
  /**
   * For a glyph that SETS rather than does — a switch over a region, a filter
   * left on. Drawn as `aria-pressed`, which is what tells a reader a control
   * they have used from one they have not: two states of one switch look
   * identical otherwise, and a glyph has no label to carry the difference.
   * Omitted on a verb, where "pressed" would claim a state the control has not
   * got.
   */
  pressed?: boolean;
  onClick?: () => void;
  /** Passed through, for a control a test already reaches by its own handle. */
  testId?: string;
}>) {
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(label);
  return (
    <span className="icon-action" ref={ref} {...trigger}>
      <Button
        iconOnly
        variant={variant}
        small={small}
        reason={reason}
        reasonId={reasonId}
        pending={pending}
        aria-label={label}
        aria-pressed={pressed}
        data-testid={testId}
        onClick={onClick}
      >
        {icon}
      </Button>
      {tip}
    </span>
  );
}
