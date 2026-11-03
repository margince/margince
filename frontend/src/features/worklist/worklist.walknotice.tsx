// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "New priorities available". A walk is frozen at its first page, so rows
// never reshuffle under the reader. Work that arrived behind them, or left,
// is reported here, and a refresh applies it. An offer, not an error: the
// rows on screen are still correct.

import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { WorklistWalk } from "./worklist.queries";
import "./worklist.css";

/**
 * The notice, where there is anything to say.
 *
 * Null on a walk that has not moved, which is the common case: a reader who
 * pages a quiet day sees nothing, and a line saying "0 new" would be noise on
 * every page they turn.
 */
export function WalkNotice({
  walk,
  onRefresh,
}: Readonly<{ walk: WorklistWalk | undefined; onRefresh: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!walk) {
    return null;
  }
  const arrived = walk.new_available ?? 0;
  const gone = walk.changed_since_snapshot;
  if (arrived === 0 && gone === 0) {
    return null;
  }
  return (
    <Callout
      kind="event"
      title={t("worklist.walk.title")}
      actions={
        <Button onClick={onRefresh}>{t("worklist.walk.refresh")}</Button>
      }
    >
      {noticeText(arrived, gone, t, locale)}
    </Callout>
  );
}

// What the notice says, in the reader's own terms.
//
// The two facts are reported SEPARATELY rather than netted off. "Three arrived
// and two left" is two things a reader can act on differently — one is a reason
// to refresh, the other explains why a count fell — and a single net figure
// would hide both behind a number that means neither.
function noticeText(
  arrived: number,
  gone: number,
  t: ReturnType<typeof useT>,
  locale: ReturnType<typeof useLocale>["locale"],
): string {
  if (arrived > 0 && gone > 0) {
    return t("worklist.walk.both", {
      arrived: formatNumber(arrived, locale),
      gone: formatNumber(gone, locale),
    });
  }
  if (arrived > 0) {
    return t("worklist.walk.arrived", {
      arrived: formatNumber(arrived, locale),
    });
  }
  return t("worklist.walk.gone", { gone: formatNumber(gone, locale) });
}
