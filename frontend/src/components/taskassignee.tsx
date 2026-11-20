// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../api/schema";
import { formatDue } from "../format/format";
import { useLocale, useT } from "../i18n";
import { useMe } from "../lib/session";
import { useViewerZone } from "../lib/viewerzone";
import { EntityRef } from "./entityref";

/**
 * Who holds an open task: "You", a colleague's name, or "Unassigned". A task on
 * a shared record is often a colleague's, so a row that leaves its holder out
 * reads as the reader's own work.
 *
 * `name: null` is the server saying it will not name the holder (a member who
 * left), so nothing is drawn. An absent `name` means the caller has none and
 * the id is looked up.
 */
export function useTaskHolder(
  id: string | null | undefined,
  name?: string | null,
): ReactNode {
  const t = useT();
  // Only a named holder can be the reader, so no id asks nothing of the session.
  const me = useMe(Boolean(id));
  if (!id) {
    return t("log.unassigned");
  }
  if (id === me.data?.user.id) {
    return t("task.assigneeYou");
  }
  if (name === null) {
    return null;
  }
  return <EntityRef kind="user" id={id} name={name} />;
}

export function TaskAssignee({
  id,
  name,
}: Readonly<{ id: string | null | undefined; name?: string | null }>) {
  const holder = useTaskHolder(id, name);
  return holder === null ? null : <span>{holder}</span>;
}

/** The parts of a task's line that have something to say, dot-separated. */
export function taskLine(parts: readonly ReactNode[]): ReactNode {
  const said = parts.filter(
    (part) =>
      part !== null && part !== undefined && part !== false && part !== "",
  );
  if (said.length === 0) {
    return undefined;
  }
  return said.slice(1).reduce(
    (line, part) => (
      <>
        {line} · {part}
      </>
    ),
    said[0],
  );
}

type TaskFacts = Pick<
  components["schemas"]["Activity"],
  "assignee_id" | "due_at" | "due_has_time" | "due_intent"
>;

/** Who holds a task and when it is due, as one line. */
export function TaskTerms({ task }: Readonly<{ task: TaskFacts }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const holder = useTaskHolder(task.assignee_id);
  return taskLine([
    holder,
    task.due_at
      ? t("co.next.due", {
          when: formatDue(
            task.due_at,
            task.due_has_time,
            locale,
            zone,
            task.due_intent,
          ),
        })
      : t("co.next.undated"),
  ]);
}
