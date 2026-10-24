// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import { Button } from "./atoms";
import { useTooltip } from "./tooltip";
import "./iconaction.css";

type IconActionFace = {
  /** The verb, translated. Spoken as the name and shown as the tip. */
  label: string;
  /**
   * One short word drawn under the glyph in a record's verb bar, where the
   * tile has no room for `label`; `label` stays the name. Hidden elsewhere.
   */
  word?: string;
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
  /** A glyph inside a line of text: the square keeps its target and takes one line box. */
  inline?: boolean;
  /** Passed through, for a control a test already reaches by its own handle. */
  testId?: string;
};

type IconActionPress = {
  href?: undefined;
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
  /**
   * For a glyph that OPENS a region rather than setting a state — the
   * derivation under a report. Drawn as `aria-expanded` with `aria-controls`
   * naming the region, which is what tells a reader both that something
   * appeared and where; `pressed` would claim a setting that stays on.
   */
  disclosure?: Readonly<{ expanded: boolean; controls: string }>;
  /** For a glyph whose press opens a dialog, so a reader hears that first. */
  opensDialog?: boolean;
  onClick?: () => void;
};

// A refused link is no link, so a refusal is drawn by the press form instead.
type IconActionLink = {
  /** Where the glyph goes (a `tel:`), drawn as a link with the same square. */
  href: string;
} & { [Key in Exclude<keyof IconActionPress, "href">]?: never };

/**
 * A verb whose glyph is its whole label: one `label` is its `aria-label` and its tooltip.
 * The handlers sit on a wrapping span, so a caller's ref and pointer events stay out of `Button`'s props.
 */
export function IconAction(
  props: Readonly<IconActionFace & (IconActionPress | IconActionLink)>,
) {
  const { label, hint, icon, inline, word } = props;
  const hintId = useId();
  const { ref, trigger, tip } = useTooltip<HTMLSpanElement>(
    hint === undefined ? label : `${label}. ${hint}`,
  );
  const face = (
    <>
      {icon}
      {word && (
        <span className="verb-word" aria-hidden="true">
          {word}
        </span>
      )}
    </>
  );
  const describedBy = hint === undefined ? undefined : hintId;
  return (
    <span
      className={inline ? "icon-action icon-action-inline" : "icon-action"}
      ref={ref}
      {...trigger}
    >
      {props.href === undefined ? (
        <Button
          iconOnly
          variant={props.variant}
          reason={props.reason}
          reasonId={props.reasonId}
          disabled={props.disabled}
          pending={props.pending}
          aria-label={label}
          aria-describedby={describedBy}
          aria-pressed={props.pressed}
          aria-expanded={props.disclosure?.expanded}
          aria-controls={props.disclosure?.controls}
          aria-haspopup={props.opensDialog ? "dialog" : undefined}
          data-testid={props.testId}
          onClick={props.onClick}
        >
          {face}
        </Button>
      ) : (
        <a
          className={`btn btn-${props.variant ?? "ghost"} btn-icon`}
          href={props.href}
          aria-label={label}
          aria-describedby={describedBy}
          data-testid={props.testId}
        >
          {face}
        </a>
      )}
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

/**
 * A verb's short tile word with its full name for a screen reader. It serves a
 * verb drawn as a plain `Button` or a popover face in the record verb bar.
 * The word shows only inside that bar.
 */
export function VerbWord({
  word,
  label,
}: Readonly<{ word: string; label: string }>) {
  return (
    <>
      <span className="verb-word" aria-hidden="true">
        {word}
      </span>
      <span className="sr-only">{label}</span>
    </>
  );
}
