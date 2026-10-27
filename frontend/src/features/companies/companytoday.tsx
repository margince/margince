// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { NextTaskSlot } from "../../components/nexttaskslot";
import { TodayPanel } from "../records/record360";
import "./company360.css";
import type { TodayReading } from "./companytoday.reading";

export { useTodayReading } from "./companytoday.reading";

export type Company360 = components["schemas"]["Company360"];

/**
 * What needs a contact: one list, the agent's finds, then the manual moves.
 * The foot carries what the list as a whole is counting down to and what the
 * advice could not show.
 */
export function NeedsList({
  reading,
  onOpenTasks,
}: Readonly<{
  reading: TodayReading;
  // Where the footer's commitment reading leads. Absent for a caller with no
  // Tasks tab of its own.
  onOpenTasks?: () => void;
}>) {
  if (reading.state !== "ready") {
    return (
      <TodayPanel
        state={reading.state}
        onOpenTasks={onOpenTasks}
        nextTask={<NextTaskSlot />}
      />
    );
  }
  return (
    <TodayPanel
      onOpenTasks={onOpenTasks}
      nextTask={<NextTaskSlot />}
      footer={reading.footer}
      notice={reading.notice}
      complete={reading.complete}
    >
      {reading.rows}
    </TodayPanel>
  );
}
