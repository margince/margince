// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What has happened to this walk since the reader started it.
//
// A walk is frozen at its first page so the order and the count hold still
// while somebody pages — that is what stops the queue moving under them. The
// two things freezing cannot hide are reported here rather than absorbed:
// work that ARRIVED behind the reader and waits for a refresh, and work that
// LEFT because it was dealt with, deleted, or is no longer theirs to see.
//
// AN OFFER, NOT AN ERROR. The day on screen is still correct — it is simply no
// longer complete, and the remedy is to refresh when the reader is ready. A
// warn tone would tell them something is wrong with a page that is working
// exactly as designed.

import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import type { WorklistWalk } from "./worklist.queries";

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
    <Callout tone="info">
      {noticeText(arrived, gone, t, locale)}{" "}
      {/* The way to act on it. Refreshing starts a new walk, which is what
          brings the arrived work in — the notice without it would name a
          problem and leave the reader to find the remedy. */}
      {arrived > 0 && (
        <Button small onClick={onRefresh}>
          {t("worklist.walk.refresh")}
        </Button>
      )}
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
