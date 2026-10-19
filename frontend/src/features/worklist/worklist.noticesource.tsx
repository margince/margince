// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The message or meeting a notice is about, opened beside the queue.
//
// An activity has no page of its own (`lib/entity.ts`), so `rowHref` gives a
// notice about one no address. It opens in the drawer for its kind instead.

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { viewerZone } from "../../format/timezone";
import { useActivity } from "../../lib/activityread";
import { OpenActivityDrawer } from "../../screens/activitydrawer";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { type WorklistItem, worklistKey } from "./worklist.queries";

/**
 * The activity a notice row may open, or undefined.
 *
 * Only a NAMED subject. The server fills `label` under the reader's grants and
 * the mail audience, and leaves it off where the reader may not read the
 * activity. So the row never offers a way into mail it would then refuse.
 */
export function noticeSourceId(item: WorklistItem): string | undefined {
  const subject = item.subject;
  if (item.source !== "notice" || subject?.type !== "activity") {
    return undefined;
  }
  return subject.label ? subject.id : undefined;
}

/** What a row needs to open its notice's source: the press, and its drawer. */
export function useNoticeSource(item: WorklistItem): Readonly<{
  open?: () => void;
  drawer: ReactNode;
}> {
  const [opened, setOpened] = useState<string | null>(null);
  const client = useQueryClient();
  const source = noticeSourceId(item);
  return {
    open: source === undefined ? undefined : () => setOpened(source),
    // Mounted only while open, so a closed drawer keeps no read that refetches
    // on every window focus, once for each row a reader ever opened.
    drawer:
      opened === null ? null : (
        <SourceDrawer
          activityId={opened}
          onClose={() => setOpened(null)}
          onReplySent={() =>
            void client.invalidateQueries({ queryKey: worklistKey })
          }
        />
      ),
  };
}

/**
 * A mail in the email drawer, anything else in the activity drawer.
 *
 * The row knows only that its subject is an activity. The kind comes from the
 * activity read, which also decides what this reader may see. Its answer goes
 * down to the drawer, so the activity is read once. A failed read goes to the
 * activity drawer, which says so and offers a retry.
 */
function SourceDrawer({
  activityId,
  onClose,
  onReplySent,
}: Readonly<{
  activityId: string;
  onClose: () => void;
  onReplySent: () => void;
}>) {
  const read = useActivity(activityId);
  const answered = read.data !== undefined || read.isError;
  const email = read.data?.kind === "email";
  const zone = viewerZone();
  return (
    <>
      <OpenEmailDrawer
        activityId={answered && email ? activityId : null}
        zone={zone}
        onClose={onClose}
        onReplySent={onReplySent}
        activity={read.data}
      />
      <OpenActivityDrawer
        activityId={answered && !email ? activityId : null}
        zone={zone}
        onClose={onClose}
        activity={read.data}
      />
    </>
  );
}
