// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The strip that chooses which body of a record is open.
//
// A record's tabs are not a segmented control. A segmented control offers a
// SETTING — two or three equal alternatives, one of which is on — and draws
// itself as a pill so it reads as one control. A record has eight bodies of
// unequal size, several of them counted, and the strip that chooses between
// them is a place a reader navigates rather than a switch they flip. Drawn as
// a pill it became a row of buttons wide enough to push the page's own content
// below the fold; drawn as a rule with the current one underlined it is the
// same shape every record tool uses, and the reader already knows it.
//
// Its own component rather than a variant of SegmentedControl: the two share
// their props and nothing else, and folding this in as a `variant` would put
// two unrelated layouts behind one name for the sake of one shared signature.

import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import "./recordtabs.css";

/**
 * RecordTabs is one row of tabs, with the current one marked.
 *
 * The strip sits between hairlines that run the full width of the window
 * rather than stopping at the record's own column. It divides the record's
 * IDENTITY from its CONTENT, and those are two halves of the page rather than
 * two blocks inside it — a rule inset to the column would read as a border
 * around the tabs.
 */
export function RecordTabs<Option extends string>({
  options,
  value,
  onChange,
  labels,
  counts,
  marks,
  label,
}: Readonly<{
  options: readonly Option[];
  value: Option;
  onChange: (next: Option) => void;
  labels: Record<Option, string>;
  // How much is behind each tab. Partial and per-option on purpose: a tab
  // whose count is absent draws none, which is what a body that is not a list
  // of things (the brief, a form) needs — and it is NOT the same as a zero. A
  // zero is a fact about the account and prints; a missing count is a fact
  // about the section and does not.
  //
  // Inside the button, so the figure joins the tab's accessible name and a
  // screen reader announces "People 10" rather than leaving it to a sighted
  // reader alone.
  counts?: Partial<Record<Option, number>>;
  // Something waits behind this tab that nobody has taken up — a lookup never
  // run, an answer never read. A dot rather than a figure, because the thing
  // waiting has no count: it is a state, and the surface behind the tab says it
  // in words. `aria-hidden`, and never the only carrier of the fact: a mark is
  // what a screen reader cannot read and a colour-blind reader may not see.
  marks?: Partial<Record<Option, boolean>>;
  // What this strip chooses BETWEEN, for a reader who meets the buttons
  // without the record around them.
  label?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <div className="recordtabs">
      <div className="recordtabs-row">
        {/* A fieldset, like every other group of controls here: a tablist
            would promise arrow-key navigation between the tabs and panels
            wired to them by id, and these are buttons that route to a page.
            A group of pressable buttons is what they are. */}
        <fieldset
          className="recordtabs-strip"
          aria-label={label ?? t("record.tabs")}
        >
          {options.map((option) => {
            const count = counts?.[option];
            return (
              <button
                key={option}
                type="button"
                className="recordtabs-tab"
                aria-pressed={option === value}
                onClick={() => onChange(option)}
              >
                {labels[option]}
                {count !== undefined && (
                  <span className="recordtabs-count t-mono">
                    {formatNumber(count, locale)}
                  </span>
                )}
                {marks?.[option] && (
                  <span className="recordtabs-mark" aria-hidden="true" />
                )}
              </button>
            );
          })}
        </fieldset>
      </div>
    </div>
  );
}
