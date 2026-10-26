// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The message or meeting a notice is about, opened beside the queue.
//
// An activity has no page of its own (`lib/entity.ts`), so `rowHref` gives a
// notice about one no address. It opens in the drawer for its kind instead.

import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { useTaskUpdate } from "../../components/taskactions";
import { viewerZone } from "../../format/timezone";
import { TASK_QUEUE_KEY } from "../../lib/activitykeys";
import { useActivity } from "../../lib/activityread";
import { useCanWrite } from "../../lib/capability";
import { OpenEmailDrawer } from "../mail/openemaildrawer";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";
import { TaskDetailModal } from "./taskdetailmodal";
import { rowHref } from "./worklist.copy";
import { type WorklistItem, worklistKey } from "./worklist.queries";

// Task and meeting row ids are activity ids; other lanes must name their activity explicitly.
export function rowActivityId(item: WorklistItem): string | undefined {
  if (
    item.source === "task" ||
    item.source === "meeting" ||
    item.source === "meeting_outcome"
  )
    return item.id;
  const subject = item.source_record ?? item.subject;
  if (subject?.type !== "activity") {
    return undefined;
  }
  return subject.id;
}

export function rowSourceUnavailable(item: WorklistItem): boolean {
  const source = rowActivityId(item);
  return (
    ((item.source_record ?? item.subject)?.type === "activity" &&
      source === undefined) ||
    (source === undefined &&
      rowHref(item) === undefined &&
      item.actions.includes("open"))
  );
}

/** Open the current activity read beside the queue, keeping its filters and scroll position. */
export function useRowSource(item: WorklistItem): Readonly<{
  open?: () => void;
  unavailable: boolean;
  drawer: ReactNode;
}> {
  const [opened, setOpened] = useState<string | null>(null);
  const client = useQueryClient();
  const source = rowActivityId(item);
  return {
    open: source === undefined ? undefined : () => setOpened(source),
    unavailable: rowSourceUnavailable(item),
    // Mounted only while open, so a closed drawer keeps no read that refetches
    // on every window focus, once for each row a reader ever opened.
    drawer:
      opened === null ? null : item.source === "task" ? (
        <OpenTaskSource
          activityId={opened}
          item={item}
          onClose={() => setOpened(null)}
        />
      ) : (
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

function OpenTaskSource({
  activityId,
  item,
  onClose,
}: Readonly<{
  activityId: string;
  item: WorklistItem;
  onClose: () => void;
}>) {
  const canUpdate = useCanWrite("activity", "update");
  const update = useTaskUpdate([worklistKey, TASK_QUEUE_KEY, ["activities"]]);
  return (
    <TaskDetailModal
      activityId={activityId}
      item={item}
      readOnly={!canUpdate}
      update={update}
      onClose={onClose}
    />
  );
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
