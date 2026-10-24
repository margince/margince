// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, useState } from "react";
import { TaskDetailDialog } from "../../components/taskactions";
import { useRecordZone } from "../../lib/recordzone";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";

export function TaskDetailModal(
  props: Omit<ComponentProps<typeof TaskDetailDialog>, "onOpenSource">,
) {
  const [source, setSource] = useState<string | null>(null);
  const zone = useRecordZone();
  return (
    <>
      <TaskDetailDialog {...props} onOpenSource={setSource} />
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
