// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { useTaskDone } from "../../components/taskactions";
import { Button } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { replyReminder } from "./deal360/dealwaiting";

type DealStatusCard = components["schemas"]["DealStatusCard"];

// Once we replied, the move opens the request's reminder. The bar marks it
// done in place, because what is left is to say nothing more is owed.
export function reminderToComplete(
  card: DealStatusCard,
  move: NonNullable<DealStatusCard["next"]>,
): Readonly<{ id: string; version: number }> | null {
  const reminder = replyReminder(card);
  return reminder &&
    move.action === "open_task" &&
    move.arguments?.activity_id === reminder.id
    ? reminder
    : null;
}

/**
 * Done on the reminder our reply leaves open, in the deal's Next bar. A reader
 * who may not write the reminder gets `fallback`, the move's own verb.
 */
export function ReminderDone({
  dealId,
  reminder,
  fallback,
}: Readonly<{
  dealId: string;
  reminder: Readonly<{ id: string; version: number }>;
  fallback: ReactNode;
}>) {
  const t = useT();
  const canUpdateTask = useCanWrite("activity", "update");
  // History draws the request's badge from the same rule, so it refreshes too.
  const done = useTaskDone(reminder.id, reminder.version, [
    ["deal-status", dealId],
    ["activities"],
  ]);
  if (!canUpdateTask) {
    return fallback;
  }
  return (
    <Button variant="primary" pending={done.pending} onClick={done.complete}>
      {t("tasks.complete")}
    </Button>
  );
}
