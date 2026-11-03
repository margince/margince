// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ComponentProps } from "react";
import { TaskDetailDialog } from "../../components/taskactions";
import { useSourceOpener } from "../../lib/opensource";
import { useRecordZone } from "../../lib/recordzone";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";
import type { WorklistItem } from "./worklist.queries";
import { WorklistWhy } from "./worklist.why";

/** A task opened where it was listed, with its source one click away. */
export function TaskDetailModal({
  item,
  ...props
}: Omit<ComponentProps<typeof TaskDetailDialog>, "openerFor" | "why"> &
  // The row the task was opened from, when it was opened from one.
  Readonly<{ item?: WorklistItem }>) {
  const openerFor = useSourceOpener();
  return (
    <TaskDetailDialog
      {...props}
      openerFor={openerFor}
      why={item && <WorklistWhy item={item} />}
    />
  );
}

/**
 * A meeting opened from the row that asks for its outcome: the meeting itself,
 * under what the row asks and the row's own verbs.
 */
export function MeetingDetailModal({
  item,
  onClose,
}: Readonly<{ item: WorklistItem; onClose: () => void }>) {
  const zone = useRecordZone();
  return (
    <OpenActivityDrawer
      activityId={item.id}
      zone={zone}
      onClose={onClose}
      about={<WorklistWhy item={item} answer />}
    />
  );
}
