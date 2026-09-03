// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PendingBody } from "../../design-system/atoms";
import type { RecordTimeline } from "../../design-system/recordtimeline";
import { useT } from "../../i18n";
import { RecordSpine, type SpineCommercial } from "./spine";
import { ThreadFailed } from "./threadfailed";
import { timelineSpineSource } from "./timelinespine";

/**
 * TimelineThread draws the thread under a call for a record whose history
 * arrives as a bare timeline page — a deal or a lead — from the page's own
 * unfiltered read.
 *
 * An empty page is a claim: "nobody has written to this record". While the
 * read is in flight that claim is false, so the thread waits rather than
 * draws it.
 *
 * A failed read is not an empty thread either: the call stands, and the
 * reader is told the thread is missing rather than shown a record nobody has
 * written to. Only a read that left NOTHING on hand is missing, though — the
 * query behind this is shared with the history tab, and a later page failing
 * there flips the same error flag while every row already read is still
 * held. Those rows are the thread; the page that did not arrive is counted
 * as "more".
 *
 * The page has no server `as_of` for these records, so the thread is read at
 * the moment it is drawn, in the reader's own clock — the same clock the
 * readings around it measure their dates against.
 */
export function TimelineThread({
  thread,
  commercial,
}: Readonly<{
  thread: RecordTimeline;
  commercial?: SpineCommercial | null;
}>) {
  const t = useT();
  if (thread.isPending) {
    return <PendingBody label={t("record.timelineLoading")} />;
  }
  if (thread.isError && thread.activities.length === 0) {
    return <ThreadFailed onRetry={thread.refetch} />;
  }
  return (
    <RecordSpine
      source={timelineSpineSource(
        thread.activities,
        new Date().toISOString(),
        thread.hasNextPage,
      )}
      commercial={commercial}
    />
  );
}
