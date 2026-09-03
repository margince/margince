// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { TagPill } from "./tagpill";
import { useTooltip } from "./tooltip";
import "./rowtags.css";

type RowTag = components["schemas"]["RowTag"];

/**
 * How many words a table cell draws before it counts the rest.
 *
 * Two, not the record panel's four. A cell shares its row with every other
 * column, and a strip that grows with the data pushes those columns around
 * until the table stops lining up — the count is what keeps the cell one width
 * whatever the row carries.
 */
const VISIBLE_ROW_TAGS = 2;

/**
 * How many tags one LIST ROW carries back at most.
 *
 * A mirror of `rowTagCap` in the store's `AttachRowTags`: a row that answered
 * with every word would turn one page of fifty rows into thousands of joined
 * rows, so the wire caps it. This tier has to know, because a strip that
 * counted the rest off the capped array would tell a record with forty tags
 * that it has five — the record page is where the whole set lives.
 */
const WIRE_ROW_TAG_CAP = 5;

/**
 * The tags a list row carries, as a chip strip.
 *
 * Passive: this cell reports, it does not act. A verb here would sit inside a
 * row that is itself a link to the record, so every press would be a guess
 * about which of the two the reader meant. Applying and removing live on the
 * record page, where the tag is the subject rather than one column of it.
 *
 * A row with no tags draws nothing at all. An empty state per row would repeat
 * the same sentence fifty times down a page, and a column of "No tags" reads as
 * a finding rather than as the absence of one.
 */
export function RowTags({ tags }: Readonly<{ tags?: readonly RowTag[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const all = tags ?? [];
  const shown = all.slice(0, VISIBLE_ROW_TAGS);
  const hidden = all.slice(VISIBLE_ROW_TAGS);
  // At the cap the wire is telling us it stopped, not that the record carries
  // exactly this many. So the strip says "more" rather than a number it cannot
  // stand behind, and names the ones it actually has.
  const capped = all.length === WIRE_ROW_TAG_CAP;
  const label = capped
    ? t("tags.moreUncounted")
    : t("tags.more", { count: formatNumber(hidden.length, locale) });
  const names = hidden.map((tag) => tag.name).join(", ");
  const tip = useTooltip<HTMLButtonElement>(
    capped ? t("tags.moreUncountedTip", { names }) : names,
  );

  if (all.length === 0) {
    return null;
  }
  return (
    <span className="rowtags">
      {shown.map((tag) => (
        <TagPill key={tag.tag_id} name={tag.name} tone={tag.color} />
      ))}
      {hidden.length > 0 && (
        // A button, not a span. The words behind the count have to be
        // reachable on FOCUS as well as on hover — an answer a pointer alone
        // can get to is an answer half the readers cannot — and `useTooltip`
        // grants no tab stop of its own, because the control it was written
        // for is a button that already has one.
        //
        // It does not navigate and it does not write: pressing it does what
        // focusing it does, which is show the words. The row underneath is a
        // link to the record, so the press is stopped here rather than opening
        // it — a reader asking what "+2" hides has not asked to leave the page.
        <button
          type="button"
          ref={tip.ref}
          className="rowtags-rest t-small"
          onClick={(event) => event.stopPropagation()}
          {...tip.trigger}
        >
          {label}
          {tip.tip}
        </button>
      )}
    </span>
  );
}
