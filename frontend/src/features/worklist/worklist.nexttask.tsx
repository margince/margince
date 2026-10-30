// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef, useState } from "react";
import { NextTaskProvider } from "../../components/nexttaskslot";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { useT } from "../../i18n";
import { navigate, parseHash, routeHash, useRoute } from "../../lib/router";
import { parseParams, useUrlParams } from "../../lib/urlstate";
import { continuedList, nextTaskHref } from "./worklist.next";
import type { Worklist } from "./worklist.queries";
import { loadedQueue, readWorklistPage } from "./worklist.reads";

/**
 * Opens the record after this one in the Worklist it came from. The list is
 * read only on the press, fresh, so a page on screen polls nothing.
 */
export function NextTaskButton() {
  // Keyed on the address, so a record opened next starts with no answer of this one's.
  return <NextTaskPress key={routeHash(useRoute())} />;
}

function NextTaskPress() {
  const t = useT();
  const [params] = useUrlParams();
  const onScreen = useRoute();
  const list = continuedList(params);
  const [reading, setReading] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  const [spent, setSpent] = useState(false);
  // Aborted when the reader leaves, so the server stops assembling a page nobody reads.
  const leaving = useRef(new AbortController());
  useEffect(() => {
    const controller = new AbortController();
    leaving.current = controller;
    return () => controller.abort();
  }, []);
  const press = async () => {
    const { signal } = leaving.current;
    setFailure(null);
    setReading(true);
    try {
      // Read on while the record on screen, or the one after it, may be on a later page.
      const pages: Worklist[] = [];
      let cursor: string | undefined;
      let next: string | undefined;
      do {
        const page = await readWorklistPage(
          list.scope,
          list.filter,
          list.owner,
          cursor,
          signal,
          list.view,
        );
        pages.push(page);
        cursor = page.next_cursor ?? undefined;
        next = nextTaskHref(
          loadedQueue(pages),
          onScreen,
          list.carried,
          cursor === undefined,
        );
      } while (!signal.aborted && next === undefined && cursor !== undefined);
      // The answer is about the record it was pressed on; a reader gone elsewhere stays there.
      if (signal.aborted) return;
      if (next === undefined) setSpent(true);
      else navigate(parseHash(next), parseParams(next));
    } catch (error) {
      if (!signal.aborted) setFailure(error);
    } finally {
      if (!signal.aborted) setReading(false);
    }
  };
  return (
    <>
      <Button
        variant="ghost"
        disabled={spent}
        pending={reading}
        onClick={() => void press()}
      >
        {spent ? t("brief.queue.nextNone") : t("brief.queue.next")}
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
