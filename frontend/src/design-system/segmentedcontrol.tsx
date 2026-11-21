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
  // How much is behind each option. An absent count draws none: a section that
  // is no list (an overview, a form) has no count, and that is not a zero.
  // The count sits inside the button, so a screen reader hears "Contacts 6".
  counts?: Partial<Record<Option, number>>;
  // Options whose count is a floor, drawn "25+": the read stopped at a bound.
  floors?: Partial<Record<Option, boolean>>;
  // The name of the `fieldset` group. A screen reader says it with each
  // option, so no option is heard out of context.
  label?: string;
  // Options with a dot: something waits behind them. The dot is `aria-hidden`,
  // so the surface the option opens states that fact in words.
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
