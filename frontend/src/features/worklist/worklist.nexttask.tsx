// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
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
  const t = useT();
  const [params] = useUrlParams();
  const onScreen = useRoute();
  const here = routeHash(onScreen);
  const list = continuedList(params);
  const [reading, setReading] = useState(false);
  const [failure, setFailure] = useState<unknown>(null);
  // Keyed on the address, so moving to another page offers the press again.
  const [spentOn, setSpentOn] = useState<string | null>(null);
  const spent = spentOn === here;
  const press = async () => {
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
        );
        pages.push(page);
        cursor = page.next_cursor ?? undefined;
        next = nextTaskHref(
          loadedQueue(pages),
          onScreen,
          list.carried,
          cursor === undefined,
        );
      } while (next === undefined && cursor !== undefined);
      if (next === undefined) setSpentOn(here);
      else navigate(parseHash(next), parseParams(next));
    } catch (error) {
      setFailure(error);
    } finally {
      setReading(false);
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
