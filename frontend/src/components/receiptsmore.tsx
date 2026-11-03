// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/button";
import { useT } from "../i18n";
import type { useHandledForYou } from "../lib/handled";

type HandledRead = Pick<
  ReturnType<typeof useHandledForYou>,
  "data" | "hasNextPage" | "fetchNextPage" | "isFetchingNextPage"
>;

/**
 * The end of a capped receipt list. It offers the next page while the server
 * has one, and otherwise says the list is not everything.
 */
export function ReceiptsMore({ handled }: Readonly<{ handled: HandledRead }>) {
  const t = useT();
  if (handled.hasNextPage) {
    return (
      <Button
        variant="link"
        pending={handled.isFetchingNextPage}
        onClick={() => void handled.fetchNextPage()}
      >
        {t("worklist.handled.showMore")}
      </Button>
    );
  }
  if (handled.data?.truncated) {
    return <p className="t-caption">{t("worklist.handled.truncated")}</p>;
  }
  return null;
}
