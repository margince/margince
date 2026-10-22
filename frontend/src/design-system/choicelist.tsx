// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId } from "react";
import { Checkbox, Radio } from "./atoms";
import "./choicelist.css";

/**
 * One answer out of a few, with every answer readable at rest. With `multiple`
 * it is any number of answers, drawn as checkboxes in the same fieldset.
 *
 * A set of radios is a question, so it carries a name and a group (a
 * `fieldset`) and neither is the caller's to forget. Without one, a screen
 * reader announces loose radios rather than one question.
 *
 * Reach for it instead of the neighbours when the reader must weigh answers:
 *
 *  - `Select` hides every option but one behind a click. Right for a long list.
 *    Wrong for a decision and for a binary.
 *  - `SegmentedControl` shows a closed set in a tab strip. Right for short
 *    labels that name a view. Wrong once an option is a sentence.
 *  - `Switch` is one thing on or off. An either/or has no off-state a reader
 *    could infer.
 *
 * `description` is part of the label, not a sibling, which is why `Radio`
 * takes a node. A help line outside the label cannot be clicked, and a screen
 * reader does not read it with the option.
 *
 * Copy never lives here: every word arrives as a prop, translated by the caller.
 */
export type Choice<Value extends string> = Readonly<{
  value: Value;
  /** The answer, written so it reads on its own. */
  label: string;
  /**
   * What choosing it means, when the label cannot carry it alone. A node so a
   * refused answer can link to what unlocks it. Keep that to one short link,
   * since its words join the answer's accessible name.
   */
  description?: ReactNode;
  /**
   * A mark that identifies the answer faster than its name does, for a choice
   * between things the reader already recognises by sight: a vendor picking its
   * own logo out of three is not reading, and making them read is slower.
   *
   * DECORATIVE, always. The label stays the answer and the accessible name, so
   * a mark that fails to load or cannot be seen costs nothing. Never the only
   * carrier of which option is which.
   */
  mark?: ReactNode;
  /**
   * Refuses this one answer while the others stay open. Its `description` says
   * why: a greyed answer with no reason is a question the reader cannot act on.
   */
  disabled?: boolean;
}>;

/**
 * One answer (the default) or several. Several come back in CHOICE order, not
 * press order, so a caller comparing two picks never sees the same set twice
 * spelled differently.
 */
type Answer<Value extends string> =
  | Readonly<{
      multiple?: false;
      /** The chosen value, or "" for a question nobody has answered yet. */
      value: Value | "";
      onChange: (next: Value) => void;
    }>
  | Readonly<{
      multiple: true;
      value: readonly Value[];
      onChange: (next: Value[]) => void;
    }>;

type Question<Value extends string> = Readonly<{
  /** The question. A group of answers with nothing naming the question is a set
   * of unrelated controls to anybody not looking at the layout. */
  legend: string;
  /**
   * Keeps the legend as the group's accessible name while taking it off screen,
   * for a caller whose surrounding layout already asks the question once — a
   * card whose heading IS the question. Visually hidden rather than dropped,
   * because the group still needs naming for a reader who cannot see that
   * heading's proximity.
   */
  hideLegend?: boolean;
  choices: readonly Choice<Value>[];
  disabled?: boolean;
  /**
   * `stack` is the default because an answer that needs explaining needs a line
   * of its own; `row` is for a closed set of SHORT labels where the options read
   * as a range — a window of days, a page size — and a column would spend four
   * rows saying what one says.
   *
   * A `row` option with a `description` is the one combination to avoid: side by
   * side, the help lines set the options' widths against each other and the
   * shorter one reads as clipped.
   *
   * `cards` gives every answer a plate of its own and draws the chosen one as a
   * selected card, for a question whose options are THINGS being chosen between
   * rather than settings: a platform, a vendor, a plan. It is where `mark`
   * earns its place, because a plate is big enough to be recognised before it
   * is read.
   */
  layout?: "stack" | "row" | "cards";
  /** Layout the surrounding form owns — the margin its own rhythm expects. It
   * lands on the fieldset, which is the only element a caller can position. */
  className?: string;
}>;

export function ChoiceList<Value extends string>(
  props: Question<Value> & Answer<Value>,
) {
  const {
    legend,
    hideLegend,
    choices,
    disabled,
    layout = "stack",
    className,
  } = props;
  // The name groups the radios, and the group is what makes arrow keys move
  // between them rather than tab. Minted here because it is a DOM detail no
  // caller should have to think about, and two ChoiceLists on one page with the
  // same hand-written name would silently become one question.
  const name = useId();
  return (
    // A `fieldset` and a `legend`, with NO `role` on top. `role="radiogroup"`
    // reads like the sharper claim and is the wrong one here: it is the role for
    // a group somebody built out of divs, and on native radio inputs the browser
    // already exposes the grouping — so the override adds nothing and takes the
    // element out of the mapping that gives `legend` its meaning. Every caller
    // that hand-rolled this reached for the ARIA pair because it had no
    // component; the component's answer is the native one.
    <fieldset
      className={["choicelist", `choicelist-${layout}`, className ?? ""]
        .filter(Boolean)
        .join(" ")}
    >
      <legend className={hideLegend ? "sr-only" : "choicelist-legend"}>
        {legend}
      </legend>
      {choices.map((choice) => {
        const shared = {
          className: "choicelist-choice",
          name,
          value: choice.value,
          disabled: disabled || choice.disabled,
          label: <ChoiceText choice={choice} />,
        };
        if (props.multiple === true) {
          const { value, onChange } = props;
          return (
            <Checkbox
              key={choice.value}
              {...shared}
              checked={value.includes(choice.value)}
              onChange={(event) => {
                const ticked = event.target.checked;
                onChange(
                  choices
                    .filter((c) =>
                      c.value === choice.value
                        ? ticked
                        : value.includes(c.value),
                    )
                    .map((c) => c.value),
                );
              }}
            />
          );
        }
        const { value, onChange } = props;
        return (
          <Radio
            key={choice.value}
            {...shared}
            checked={choice.value === value}
            onChange={() => onChange(choice.value)}
          />
        );
      })}
    </fieldset>
  );
}

function ChoiceText<Value extends string>({
  choice,
}: Readonly<{ choice: Choice<Value> }>) {
  return (
    <span className="choicelist-text">
      {choice.mark === undefined ? null : (
        <span className="choicelist-mark" aria-hidden="true">
          {choice.mark}
        </span>
      )}
      <span className="choicelist-label">{choice.label}</span>
      {choice.description != null && <span>{choice.description}</span>}
    </span>
  );
}
