// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import "./stack.css";

/**
 * SPACE BETWEEN TWO THINGS, from the scale, without a stylesheet.
 *
 * Core screens have never needed this: a screen file may write a class and the
 * sheet beside it. An EXTENSION unit may not — nothing under `extensions/*` /
 * `frontend` imports CSS and the bundler gives a unit nowhere to put any — so
 * without a published primitive a unit's only two ways to separate a name from
 * the button that removes it are a bare `<div>` (no gap at all: the two render
 * jammed together) or a core class name copied out of `atoms.css`, which is a
 * promise nobody made and which a rename breaks silently in a tree the renamer
 * never opens.
 *
 * So the point is not that a flexbox is hard. It is that the SCALE is the
 * product: a unit that could set its own spacing would set 6px, 10px, 14px, and
 * a screen assembled from core cards and unit rows would be spaced by two
 * different systems. `gap` names a step, and the steps are the tokens.
 *
 * It draws NOTHING — no ground, no border, no padding, no typography. A box a
 * reader can see is a `Card` or a `Panel`, and a layout primitive that grew a
 * background would be a second one of those under a name that hides it.
 *
 * `Row` is the same primitive lying down, and it is a separate export rather
 * than a `direction` prop because the two differ in what else they must decide:
 * a row has to say what happens when it does not fit and how its items line up
 * against each other, and a stack has neither question.
 */

/** A step on the space scale — the tokens, not pixels. */
export type SpaceStep = "1" | "2" | "3" | "4" | "6";

type StackProps = Readonly<{
  /** Space between children. Defaults to `3`, the field-to-field step. */
  gap?: SpaceStep;
  children: ReactNode;
}>;

/** Children in a column, evenly separated. */
export function Stack({ gap = "3", children }: StackProps) {
  return <div className={`ds-stack ds-gap-${gap}`}>{children}</div>;
}

type RowProps = StackProps &
  Readonly<{
    /**
     * How the children line up across the row. `center` is the default because
     * a row is nearly always a label beside a control; `start` is for a row
     * whose items are different heights and should share a top edge; `baseline`
     * aligns text of different sizes on the line it is read from.
     */
    align?: "center" | "start" | "baseline";
    /**
     * Where the free space goes. `between` is the one that earns its name: a
     * label on the left and its verb on the right is the shape every card
     * footer has, and a unit writing it by hand reaches for `margin-left:auto`
     * on a child it may not style.
     */
    justify?: "start" | "end" | "between";
    /**
     * Whether the row wraps when it does not fit. Defaults to true: a row of
     * chips or verbs on a narrow screen has to go somewhere, and off-screen is
     * the one answer that loses the content.
     */
    wrap?: boolean;
  }>;

/** Children in a line, evenly separated, wrapping by default. */
export function Row({
  gap = "2",
  align = "center",
  justify = "start",
  wrap = true,
  children,
}: RowProps) {
  const classes = [
    "ds-row",
    `ds-gap-${gap}`,
    `ds-align-${align}`,
    `ds-justify-${justify}`,
    wrap ? "ds-row-wrap" : "ds-row-nowrap",
  ];
  return <div className={classes.join(" ")}>{children}</div>;
}
