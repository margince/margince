// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { itemTitle } from "./worklist.copy";
import type { Worklist } from "./worklist.queries";

// What has happened since the night looked.
//
// The morning is assembled overnight and read hours later, and the gap is where
// a rep gets caught out: a buyer replies at 07:15, the page still reflects 06:00,
// and the row that changed sits in the same position saying the same thing. The
// feed below is correctly ordered either way — the ranking is live — but nothing
// on the page said "this is new since the brief".
//
// THE SERVER DECIDES WHAT IS NEW, not this. Each row carries
// `changed_since_brief`, stamped against the run's own DATA CUTOFF rather than
// the later instant it finished writing — a run that read at 06:00 and wrote at
// 06:42 has a 42-minute window, and the wrong instant hides exactly the reply a
// rep opens the page to find. The browser cannot make that comparison: it does
// not know which of a row's several timestamps is the material one.
//
// ABSENT IS NOT FALSE. A row carries no flag at all when there was no run to
// compare against, and this strip draws nothing in that case rather than
// reporting a morning where nothing changed. "The night saw this" and "there was
// no night" are different facts, and a reader who cannot tell them apart will
// trust the wrong one.

/** How many titles the strip names before it counts the rest. */
const NAMED = 3;

/**
 * What changed since the overnight run, or nothing.
 *
 * Drawn only when something did. A strip that said "nothing has changed" every
 * morning would teach a reader to stop reading it, and on a day with no run it
 * would be saying something it cannot know.
 */
export function ChangedSinceBrief({ day }: Readonly<{ day: Worklist }>) {
  const t = useT();
  const { locale } = useLocale();
  const changed = (day.queue ?? []).filter(
    (item) => item.changed_since_brief === true,
  );
  if (changed.length === 0) {
    return null;
  }
  const named = changed.slice(0, NAMED);
  const rest = changed.length - named.length;
  return (
    <Callout tone="info">
      {t("brief.changed.lead")}{" "}
      {named.map((item) => itemTitle(item, t, locale)).join(" · ")}
      {rest > 0
        ? ` · ${t("brief.changed.more", {
            count: formatNumber(rest, locale),
          })}`
        : ""}{" "}
      <a className="entity-link" href="#/worklist">
        {t("brief.changed.open")}
      </a>
    </Callout>
  );
}
