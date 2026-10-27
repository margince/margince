// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { NextTaskProvider } from "../../components/nexttaskslot";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { navigate, parseHash, useRoute } from "../../lib/router";
import { parseParams, useUrlParams } from "../../lib/urlstate";
import { continuedList, nextTaskHref } from "./worklist.next";
import type { Worklist } from "./worklist.queries";
import { loadedQueue, useWorklist } from "./worklist.reads";

/** Opens the next open row's record in the Worklist this record came from,
 *  read fresh on the press so a finished task is gone and an urgent row leads. */
export function NextTaskButton() {
  const t = useT();
  const [params] = useUrlParams();
  const onScreen = useRoute();
  const list = continuedList(params);
  // The drawer's own read and key, so the two share one cache of the list.
  const day = useWorklist(
    list.scope,
    list.filter,
    list.owner === "" ? undefined : list.owner,
  );
  const [reading, setReading] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  const nextIn = (pages: readonly Worklist[] | undefined) =>
    nextTaskHref(loadedQueue(pages ?? []), onScreen, list.carried);
  const done =
    day.isSuccess && !day.hasNextPage && nextIn(day.data.pages) === undefined;
  const press = async () => {
    setFailure(null);
    setReading(true);
    try {
      // Read on page by page until a row on another record turns up. A failed
      // read opens nothing: the cache it would fall back on is the stale list.
      let read = await day.refetch();
      while (!read.isError) {
        const next = nextIn(read.data?.pages);
        if (next !== undefined) {
          navigate(parseHash(next), parseParams(next));
          return;
        }
        // A last page with no cursor is the end of the list.
        if (!read.data?.pages.at(-1)?.next_cursor) return;
        read = await day.fetchNextPage();
      }
      setFailure(read.error);
    } finally {
      setReading(false);
    }
  };
  return (
    <>
      <Button
        variant="ghost"
        disabled={done}
        pending={reading}
        onClick={() => void press()}
      >
        {done ? t("brief.queue.nextDone") : t("brief.queue.next")}
      </Button>
      <ErrorLine error={failure} inline />
    </>
  );
}

const nextTask = <NextTaskButton />;

/** Lets every record page draw `NextTaskButton` through `NextTaskSlot`. */
export function NextTaskHost({ children }: Readonly<{ children: ReactNode }>) {
  return <NextTaskProvider value={nextTask}>{children}</NextTaskProvider>;
}
