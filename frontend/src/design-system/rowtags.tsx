// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { TagPill } from "./tagpill";
import "./rowtags.css";

type RowTag = components["schemas"]["RowTag"];

/**
 * How many words a table cell draws before it counts the rest.
 *
 * Two, not the record panel's four. A cell shares its row with every other
 * column, and a strip that grows with the data pushes those columns around
 * until the table stops lining up — the count is what keeps the cell one
 * width whatever the row carries.
 */
const VISIBLE_ROW_TAGS = 2;

/**
 * The tags a list row carries, as a chip strip.
 *
 * Passive: this cell reports, it does not act. A verb here would sit inside a
 * row that is itself a link to the record, so every press would be a guess
 * about which of the two the reader meant. Applying and removing live on the
 * record page, where the tag is the subject rather than one column of it.
 *
 * A row with no tags draws nothing at all. An empty state per row would repeat
 * the same sentence fifty times down a page, and a column of "No tags" reads
 * as a finding rather than as the absence of one.
 */
export function RowTags({ tags }: Readonly<{ tags?: readonly RowTag[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const all = tags ?? [];
  if (all.length === 0) {
    return null;
  }
  const shown = all.slice(0, VISIBLE_ROW_TAGS);
  const rest = all.length - shown.length;
  return (
    <span className="rowtags">
      {shown.map((tag) => (
        <TagPill key={tag.tag_id} name={tag.name} tone={tag.color} />
      ))}
      {rest > 0 && (
        // The words themselves, not just the number: a reader who has to open
        // the record to learn what "+3" hides has been given a puzzle rather
        // than an answer.
        <span
          className="rowtags-rest t-small"
          title={all
            .slice(VISIBLE_ROW_TAGS)
            .map((tag) => tag.name)
            .join(", ")}
        >
          {t("tags.more", { count: formatNumber(rest, locale) })}
        </span>
      )}
    </span>
  );
}
