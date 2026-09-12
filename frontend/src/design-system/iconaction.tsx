// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
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
  hint,
  icon,
  variant,
  small,
  reason,
  reasonId,
  disabled,
  pending,
  pressed,
  onClick,
  testId,
}: Readonly<{
  /** The verb, translated. Spoken as the name and shown as the tip. */
  label: string;
  /**
   * What the verb DOES, where the word alone cannot say it.
   *
   * A glyph verb answers "what is this" with its label and stops there. Some
   * verbs need a second sentence — a pin is a personal ordering preference that
   * holds until it is undone, and none of that is in the word "Pin".
   *
   * It joins the TIP and becomes the accessible DESCRIPTION, never part of the
   * name: a control list that read "Pin to the top of your own worklist, only
   * you see it…" once per row would be worse than the bare word. Omitted, the
   * control is exactly what it was.
   */
  hint?: string;
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
  /**
   * A precondition this verb is waiting on, with no sentence to go with it —
   * the third refusal `Button` already draws and the one a glyph most often
   * needs: a row's quiet verbs held while the write its primary started is out.
   * `pending` would be the wrong spelling of that (it claims a write THIS
   * control started) and `reason` the wrong one too (it owes the reader a
   * sentence, and the fact here is simply "not yet").
   */
  disabled?: boolean;
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
  const hintId = useId();
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(
    hint === undefined ? label : `${label}. ${hint}`,
  );
  return (
    <span className="icon-action" ref={ref} {...trigger}>
      <Button
        iconOnly
        variant={variant}
        small={small}
        reason={reason}
        reasonId={reasonId}
        disabled={disabled}
        pending={pending}
        aria-label={label}
        aria-describedby={hint === undefined ? undefined : hintId}
        aria-pressed={pressed}
        data-testid={testId}
        onClick={onClick}
      >
        {icon}
      </Button>
      {tip}
      {/* The description a screen reader reads AFTER the name, and the one a
          pointer gets from the tip above. Visually hidden because the row has
          no space for it — the tip is where a sighted reader meets it. */}
      {hint !== undefined && (
        <span id={hintId} className="sr-only">
          {hint}
        </span>
      )}
    </span>
  );
}
