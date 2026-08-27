// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A row of pills that narrows a list to one of its parts.
//
// Not a SegmentedControl. That control offers a setting — two or three equal
// alternatives, drawn as one joined object because exactly one of them is on.
// These are cuts THROUGH a list: each says how much of the list it holds, the
// first one holds all of it, and a reader picks one to look at rather than to
// set. Joined into a single object the counts read as part of one label, and
// the row stopped saying what it is for.

import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import "./filterpills.css";

export type FilterPill<Value extends string> = {
  value: Value;
  label: string;
  // How much this cut holds. Absent draws no number — which is NOT a zero: a
  // list still loading, or one whose server said `has_more`, knows only a
  // floor, and a floor printed as a count is a wrong number rather than a
  // missing one.
  count?: number;
};

/**
 * FilterPills is the row, with the current cut marked.
 *
 * The count rides inside the button so it joins the accessible name — a screen
 * reader announces "Conversations 5" rather than leaving the figure to a
 * sighted reader alone.
 */
export function FilterPills<Value extends string>({
  pills,
  value,
  onChange,
  label,
}: Readonly<{
  pills: readonly FilterPill<Value>[];
  value: Value;
  onChange: (next: Value) => void;
  // What this row cuts, for a reader who meets the buttons without the list.
  label?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <fieldset className="filterpills" aria-label={label ?? t("filter.label")}>
      {pills.map((pill) => (
        <button
          key={pill.value}
          type="button"
          className="filterpill"
          aria-pressed={pill.value === value}
          onClick={() => onChange(pill.value)}
        >
          {pill.label}
          {pill.count !== undefined && (
            <span className="filterpill-count t-mono">
              {formatNumber(pill.count, locale)}
            </span>
          )}
        </button>
      ))}
    </fieldset>
  );
}
