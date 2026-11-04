// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { useMe } from "../lib/session";
import { EntityRef } from "./entityref";

/**
 * Who holds an open task: "You", a colleague's name, or "Unassigned". A task on
 * a shared record is often a colleague's, so a row that leaves its holder out
 * reads as the reader's own work.
 */
export function TaskAssignee({
  id,
  name,
}: Readonly<{ id: string | null | undefined; name?: string | null }>) {
  const t = useT();
  const me = useMe();
  if (!id) {
    return <span>{t("log.unassigned")}</span>;
  }
  if (id === me.data?.user.id) {
    return <span>{t("task.assigneeYou")}</span>;
  }
  return <EntityRef kind="user" id={id} name={name} />;
}
