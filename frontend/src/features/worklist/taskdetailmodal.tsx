// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, useState } from "react";
import { TaskDetailDialog } from "../../components/taskactions";
import { useRecordZone } from "../../lib/recordzone";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";
import type { WorklistItem } from "./worklist.queries";
import { WorklistWhy } from "./worklist.why";

export function TaskDetailModal({
  item,
  ...props
}: Omit<ComponentProps<typeof TaskDetailDialog>, "onOpenSource" | "why"> &
  // The row the task was opened from, when it was opened from one.
  Readonly<{ item?: WorklistItem }>) {
  const [source, setSource] = useState<string | null>(null);
  const zone = useRecordZone();
  return (
    <>
      <TaskDetailDialog
        {...props}
        onOpenSource={setSource}
        why={item && <WorklistWhy item={item} />}
      />
      {source && (
        <OpenActivityDrawer
          activityId={source}
          zone={zone}
          onClose={() => setSource(null)}
        />
      )}
    </>
  );
}
