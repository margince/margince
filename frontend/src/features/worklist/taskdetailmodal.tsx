// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ComponentProps, useState } from "react";
import { searchEmailRoute } from "../../lib/activityroute";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";
import { TaskDetailDialog } from "./taskactions";

export function TaskDetailModal(
  props: Omit<
    ComponentProps<typeof TaskDetailDialog>,
    "onOpenSource" | "onOpenEmail"
  >,
) {
  const [source, setSource] = useState<string | null>(null);
  const zone = useRecordZone();
  return (
    <>
      {source === null && (
        <TaskDetailDialog
          {...props}
          onOpenSource={setSource}
          onOpenEmail={(id, subject) => {
            props.onClose();
            navigate(searchEmailRoute(subject, id));
          }}
        />
      )}
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
