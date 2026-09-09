// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { useT } from "../i18n";

// What the queue page says about ITSELF, above the three groups.
//
// One slot in practice: the refused write is computed only when there is no
// skew, so a reader never meets both. They keep separate headings because they
// are separate claims — one says the list is stale, the other that a write the
// reader made did not land.

/**
 * A 409 the reader can re-read past.
 *
 * Re-reading is the only move that clears it, so the notice carries the verb
 * rather than leaving it to the page's own chrome.
 */
export function QueueSkewNotice({
  message,
  onReload,
}: Readonly<{ message: string; onReload: () => void }>) {
  const t = useT();
  return (
    <Callout
      kind="outcome"
      tone="warn"
      title={t("sched.skewTitle")}
      actions={
        <Button small onClick={onReload}>
          {t("sched.reload")}
        </Button>
      }
    >
      {message}
    </Callout>
  );
}

/** A move or a withdrawal the server refused, in the server's own words. */
export function QueueWriteRefused({ message }: Readonly<{ message: string }>) {
  const t = useT();
  return (
    <Callout kind="outcome" tone="danger" title={t("sched.writeFailed")}>
      {message}
    </Callout>
  );
}
