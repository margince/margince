// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Upcoming and backlog: planned work due after today, at the end of Today. It
// is its own read (`filter=upcoming`), opened by the reader, and its count
// never joins Today's outstanding total.

import { useState } from "react";
import { Button, OptionCount } from "../../design-system/atoms";
import { Disclosure } from "../../design-system/disclosure";
import { ErrorLine } from "../../design-system/errorline";
import { ScrollSentinel } from "../../design-system/scrollsentinel";
import { useT } from "../../i18n";
import { exactCount } from "../../lib/worklistcounts";
import { UPCOMING } from "../../lib/worklistviews";
import { NO_PICKS } from "./worklist.bulkdone";
import type { Worklist, WorklistScope } from "./worklist.queries";
import { loadedQueue, useWorklist } from "./worklist.queries";
import { QueueRows, type RowContext } from "./worklist.queue";
import { rowIdentity } from "./worklist.rowidentity";

/** The section's count, or null when the day holds no upcoming work at all. */
function upcomingCount(day: Worklist): Readonly<{ exact?: number }> | null {
  const count = day.counts.find((entry) => entry.category === UPCOMING);
  if (!count) return null;
  return { exact: exactCount(count) };
}

export function UpcomingSection({
  day,
  scope,
  owner,
  rows,
}: Readonly<{
  day: Worklist;
  scope: WorklistScope;
  owner: string;
  rows: RowContext;
}>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const read = useWorklist(
    scope,
    UPCOMING,
    owner === "" ? undefined : owner,
    open,
    "today",
  );
  const count = upcomingCount(day);
  if (count === null) return null;
  const items = read.data ? loadedQueue(read.data.pages) : [];
  const positions = new Map(
    items.map((item, at) => [rowIdentity(item), at] as const),
  );
  return (
    <Disclosure
      onToggle={setOpen}
      summary={
        <span className="worklist-upcoming-head">
          {t("worklist.filter.upcoming")}
          {count.exact !== undefined && <OptionCount count={count.exact} />}
        </span>
      }
    >
      {items.length > 0 && (
        // A look-ahead, not today's work: its rows take no part in selection.
        <QueueRows
          items={items}
          {...rows}
          positions={positions}
          picks={NO_PICKS}
        />
      )}
      {read.data && items.length === 0 && !read.hasNextPage && (
        <p className="t-body">{t("worklist.upcoming.empty")}</p>
      )}
      {read.isPending && open && (
        <p className="t-caption">{t("scrollSentinel.loading")}</p>
      )}
      {read.isError && !read.data && (
        <ErrorLine
          actions={
            <Button onClick={() => void read.refetch()}>
              {t("common.retry")}
            </Button>
          }
        >
          {t("worklist.upcoming.failed")}
        </ErrorLine>
      )}
      {read.data && (
        <ScrollSentinel
          list={`upcoming/${scope}/${owner}`}
          cursor={read.data?.pages.at(-1)?.next_cursor ?? undefined}
          loading={read.isFetchingNextPage}
          failed={read.isFetchNextPageError}
          onLoadMore={() => void read.fetchNextPage()}
          end={items.length > 0 ? t("worklist.upcoming.end") : undefined}
        />
      )}
    </Disclosure>
  );
}
