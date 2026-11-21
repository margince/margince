// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef } from "react";
import { OptionCount } from "./optioncount";
import { useWrapped } from "./segmentedcontrol.wrap";
import "./atoms.css";

export function SegmentedControl<Option extends string>({
  options,
  value,
  onChange,
  labels,
  counts,
  floors,
  label,
  marks,
  busy,
  refused,
  reasonId,
}: Readonly<{
  options: readonly Option[];
  value: Option;
  onChange: (next: Option) => void;
  labels: Record<Option, string>;
  // How much is behind each option, for a strip that chooses between bodies of
  // a record rather than between settings. Partial and per-option on purpose:
  // an option whose count is absent draws none, which is what a section that
  // is not a list of things (an overview, a form) needs — and it is NOT the
  // same as a zero. A zero is a fact about the account and prints; a missing
  // count is a fact about the section and does not.
  //
  // Inside the button, so the count joins the option's accessible name and a
  // screen reader announces "Contacts 6" rather than leaving the figure to a
  // sighted reader alone.
  counts?: Partial<Record<Option, number>>;
  // Options whose count is a floor, drawn "25+": the read stopped at a bound.
  floors?: Partial<Record<Option, boolean>>;
  // Accessible name for the control as a whole (the `fieldset` group); a
  // screen reader announces it alongside each option so the buttons aren't
  // read out of context. Optional so existing callers are unaffected.
  label?: string;
  // Options carrying a dot: something waits behind that option. Decorative by
  // construction — the dot is `aria-hidden` and the fact it hints at must be
  // stated in words on the surface the option opens, because a mark is the one
  // thing a screen reader cannot read out and a colour-blind reader may not
  // see. It draws attention; it never carries the meaning alone.
  marks?: Partial<Record<Option, boolean>>;
  // The option whose choice is being stored. Every option waits until it lands,
  // so a second press cannot overtake the first.
  busy?: Option;
  // Options the reader may not choose now, such as an hour already past. Each
  // stays in view, so the set keeps its shape.
  refused?: Partial<Record<Option, boolean>>;
  // The id of the one sentence that says why, which every refused option
  // points at. The caller prints it once, under the strip.
  reasonId?: string;
}>) {
  const strip = useRef<HTMLFieldSetElement>(null);
  const wrapped = useWrapped(strip);
  return (
    <fieldset
      ref={strip}
      className="segmented"
      aria-label={label}
      disabled={busy !== undefined}
      data-wrapped={wrapped || undefined}
    >
      {options.map((option) => {
        const count = counts?.[option];
        const barred = refused?.[option] === true;
        return (
          <button
            key={option}
            type="button"
            aria-pressed={option === value}
            aria-busy={option === busy || undefined}
            disabled={barred}
            data-refused={barred || undefined}
            aria-describedby={barred ? reasonId : undefined}
            onClick={() => onChange(option)}
          >
            {labels[option]}
            {count !== undefined && (
              <OptionCount count={count} floor={floors?.[option]} />
            )}
            {marks?.[option] && <span className="segmented-mark" aria-hidden />}
          </button>
        );
      })}
    </fieldset>
  );
}
